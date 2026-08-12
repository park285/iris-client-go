package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestH2CClientFetchMediaChunkPostsSignedBotControlRequest(t *testing.T) {
	var (
		gotMethod      string
		gotPath        string
		gotContentType string
		gotSignature   string
		gotBodyHash    string
		gotRequest     MediaChunkRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotBodyHash = r.Header.Get(HeaderIrisBodySHA256)
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if err := json.NewEncoder(w).Encode(MediaChunkResponse{
			ChunkBase64: "AAE=",
			TotalLength: 2,
			MIMEType:    "image/png",
			SHA256:      strings.Repeat("a", 64),
			EOF:         true,
			MediaCount:  1,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	request := validMediaChunkRequest()
	client := NewH2CClient(server.URL, "unused-token",
		WithBotControlToken("bot-control-secret"),
		WithHTTPClient(server.Client()),
	)
	response, err := client.FetchMediaChunk(t.Context(), request)
	if err != nil {
		t.Fatalf("FetchMediaChunk() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != PathMediaChunk {
		t.Fatalf("path = %q, want %q", gotPath, PathMediaChunk)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotSignature == "" || gotBodyHash == "" {
		t.Fatalf("HMAC headers missing: signature=%q bodyHash=%q", gotSignature, gotBodyHash)
	}
	if gotRequest != request {
		t.Fatalf("request body = %+v, want %+v", gotRequest, request)
	}
	if response == nil || response.ChunkBase64 != "AAE=" || response.TotalLength != 2 || response.MIMEType != "image/png" || response.SHA256 != strings.Repeat("a", 64) || !response.EOF || response.MediaCount != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func TestH2CClientFetchMediaChunkValidatesBeforeTransport(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-token", WithHTTPClient(server.Client()))
	base := validMediaChunkRequest()
	tests := []struct {
		name   string
		mutate func(*MediaChunkRequest)
		want   string
	}{
		{name: "blank message id", mutate: func(req *MediaChunkRequest) { req.MessageID = " " }, want: "messageId must not be blank"},
		{name: "negative source generation", mutate: func(req *MediaChunkRequest) { req.SourceGenerationID = -1 }, want: "sourceGenerationId must be non-negative"},
		{name: "blank chat id", mutate: func(req *MediaChunkRequest) { req.ChatID = " " }, want: "chatId must not be blank"},
		{name: "chat id leading zero", mutate: func(req *MediaChunkRequest) { req.ChatID = "01" }, want: "chatId must be a canonical positive decimal"},
		{name: "chat id sign", mutate: func(req *MediaChunkRequest) { req.ChatID = "+1" }, want: "chatId must be a canonical positive decimal"},
		{name: "chat id overflow", mutate: func(req *MediaChunkRequest) { req.ChatID = "9223372036854775808" }, want: "chatId must be a canonical positive decimal"},
		{name: "chat log id whitespace", mutate: func(req *MediaChunkRequest) { req.ChatLogID = " 5" }, want: "chatLogId must be a canonical positive decimal"},
		{name: "chat log id zero", mutate: func(req *MediaChunkRequest) { req.ChatLogID = "0" }, want: "chatLogId must be a canonical positive decimal"},
		{name: "unsupported type", mutate: func(req *MediaChunkRequest) { req.Type = "23" }, want: "type must be one of"},
		{name: "negative media index", mutate: func(req *MediaChunkRequest) { req.MediaIndex = -1 }, want: "mediaIndex must be between"},
		{name: "media index too large", mutate: func(req *MediaChunkRequest) { req.MediaIndex = 10 }, want: "mediaIndex must be between"},
		{name: "negative offset", mutate: func(req *MediaChunkRequest) { req.Offset = -1 }, want: "offset must be non-negative"},
		{name: "zero length", mutate: func(req *MediaChunkRequest) { req.Length = 0 }, want: "length must be between"},
		{name: "length too large", mutate: func(req *MediaChunkRequest) { req.Length = mediaMaxChunkBytes + 1 }, want: "length must be between"},
		{name: "offset and length overflow", mutate: func(req *MediaChunkRequest) { req.Offset = math.MaxInt64 }, want: "offset plus length overflows int64"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			_, err := client.FetchMediaChunk(t.Context(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FetchMediaChunk() error = %v, want %q", err, test.want)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("invalid requests reached transport %d times", got)
	}
}

func TestH2CClientFetchMediaChunkRejectsSemanticallyInvalidResponse(t *testing.T) {
	tests := []struct {
		name           string
		mutateRequest  func(*MediaChunkRequest)
		mutateResponse func(*MediaChunkResponse)
		want           string
	}{
		{name: "zero total length", mutateResponse: func(response *MediaChunkResponse) { response.TotalLength = 0 }, want: "totalLength must be between"},
		{name: "total length too large", mutateResponse: func(response *MediaChunkResponse) { response.TotalLength = mediaMaxTotalBytes + 1 }, want: "totalLength must be between"},
		{name: "zero media count", mutateResponse: func(response *MediaChunkResponse) { response.MediaCount = 0 }, want: "mediaCount must be between"},
		{name: "media count too large", mutateResponse: func(response *MediaChunkResponse) { response.MediaCount = mediaMaxCount + 1 }, want: "mediaCount must be between"},
		{name: "media index outside response count", mutateRequest: func(request *MediaChunkRequest) { request.MediaIndex = 1 }, want: "outside mediaCount"},
		{name: "empty MIME", mutateResponse: func(response *MediaChunkResponse) { response.MIMEType = "" }, want: "mimeType is invalid"},
		{name: "MIME too long", mutateResponse: func(response *MediaChunkResponse) { response.MIMEType = strings.Repeat("a", 127) + "/b" }, want: "mimeType is invalid"},
		{name: "MIME non ASCII", mutateResponse: func(response *MediaChunkResponse) { response.MIMEType = "image/😀" }, want: "mimeType is invalid"},
		{name: "MIME invalid token", mutateResponse: func(response *MediaChunkResponse) { response.MIMEType = "image/png;private" }, want: "mimeType is invalid"},
		{name: "uppercase SHA", mutateResponse: func(response *MediaChunkResponse) { response.SHA256 = strings.Repeat("A", 64) }, want: "sha256 must be lowercase"},
		{name: "short SHA", mutateResponse: func(response *MediaChunkResponse) { response.SHA256 = strings.Repeat("a", 63) }, want: "sha256 must be lowercase"},
		{name: "non hexadecimal SHA", mutateResponse: func(response *MediaChunkResponse) { response.SHA256 = strings.Repeat("g", 64) }, want: "sha256 must be lowercase"},
		{name: "empty chunk", mutateResponse: func(response *MediaChunkResponse) { response.ChunkBase64 = "" }, want: "decoded chunk length must be between"},
		{name: "unpadded base64", mutateResponse: func(response *MediaChunkResponse) { response.ChunkBase64 = "AAE" }, want: "canonical base64"},
		{name: "noncanonical padding bits", mutateResponse: func(response *MediaChunkResponse) { response.ChunkBase64 = "AAF=" }, want: "canonical base64"},
		{name: "decoded chunk exceeds request", mutateRequest: func(request *MediaChunkRequest) { request.Length = 1 }, mutateResponse: func(response *MediaChunkResponse) { response.ChunkBase64 = "AAE=" }, want: "decoded chunk length must be between"},
		{name: "chunk exceeds total", mutateResponse: func(response *MediaChunkResponse) { response.TotalLength = 1 }, want: "beyond totalLength"},
		{name: "EOF mismatch", mutateResponse: func(response *MediaChunkResponse) { response.TotalLength = 3 }, want: "eof does not match"},
		{name: "short nonfinal chunk", mutateRequest: func(request *MediaChunkRequest) { request.Length = 2 }, mutateResponse: func(response *MediaChunkResponse) {
			response.ChunkBase64 = "AA=="
			response.TotalLength = 4
			response.EOF = false
		}, want: "non-final chunk length must equal"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validMediaChunkRequest()
			if test.mutateRequest != nil {
				test.mutateRequest(&request)
			}
			response := validMediaChunkResponse()
			if test.mutateResponse != nil {
				test.mutateResponse(&response)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Fatalf("encode response: %v", err)
				}
			}))
			defer server.Close()

			client := NewH2CClient(server.URL, "unused-token", WithHTTPClient(server.Client()))
			if _, err := client.FetchMediaChunk(t.Context(), request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FetchMediaChunk() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestH2CClientFetchMediaChunkTrimsOpaqueMessageID(t *testing.T) {
	var got MediaChunkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(MediaChunkResponse{
			ChunkBase64: "AA==",
			TotalLength: 1,
			MIMEType:    "image/png",
			SHA256:      strings.Repeat("a", 64),
			EOF:         true,
			MediaCount:  1,
		})
	}))
	defer server.Close()

	request := validMediaChunkRequest()
	request.MessageID = "  message-1  "
	client := NewH2CClient(server.URL, "unused-token", WithHTTPClient(server.Client()))
	if _, err := client.FetchMediaChunk(t.Context(), request); err != nil {
		t.Fatalf("FetchMediaChunk() error = %v", err)
	}
	if got.MessageID != "message-1" {
		t.Fatalf("messageId = %q, want trimmed opaque identity", got.MessageID)
	}
}

func TestH2CClientFetchMediaChunkPropagatesContextAndTransportErrors(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); !errors.Is(err, context.Canceled) {
			return nil, errors.New("request context was not canceled")
		}
		return nil, context.Canceled
	})
	client := NewH2CClient("http://localhost", "unused-token", WithRoundTripper(rt))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.FetchMediaChunk(ctx, validMediaChunkRequest())
	if err == nil {
		t.Fatal("FetchMediaChunk() error = nil, want context/transport error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestH2CClientFetchMediaChunkRejectsResponseSchemaDrift(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "unknown field",
			body: `{"chunkBase64":"AA==","totalLength":1,"mimeType":"image/png","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","eof":true,"mediaCount":1,"path":"/private/cache"}`,
		},
		{
			name: "missing media count",
			body: `{"chunkBase64":"AA==","totalLength":1,"mimeType":"image/png","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","eof":true}`,
		},
		{
			name: "duplicate field",
			body: `{"chunkBase64":"AA==","totalLength":1,"mimeType":"image/png","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","eof":true,"mediaCount":1,"mediaCount":1}`,
		},
		{
			name: "trailing JSON",
			body: `{"chunkBase64":"AA==","totalLength":1,"mimeType":"image/png","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","eof":true,"mediaCount":1}{}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			client := NewH2CClient(server.URL, "unused-token", WithHTTPClient(server.Client()))
			if _, err := client.FetchMediaChunk(t.Context(), validMediaChunkRequest()); err == nil {
				t.Fatal("FetchMediaChunk() error = nil, want strict response rejection")
			}
		})
	}
}

func validMediaChunkRequest() MediaChunkRequest {
	return MediaChunkRequest{
		MessageID:          "message-1",
		SourceGenerationID: 0,
		RawSourceLogID:     18,
		SourceLogID:        19,
		ChatID:             "9223372036854775807",
		ChatLogID:          "9223372036854775806",
		Type:               mediaTypeImage,
		MediaIndex:         0,
		Offset:             0,
		Length:             mediaMaxChunkBytes,
	}
}

func validMediaChunkResponse() MediaChunkResponse {
	return MediaChunkResponse{
		ChunkBase64: "AAE=",
		TotalLength: 2,
		MIMEType:    "image/png",
		SHA256:      strings.Repeat("a", 64),
		EOF:         true,
		MediaCount:  1,
	}
}
