package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestNewH2CClientDefaults(t *testing.T) {
	c := NewH2CClient(" http://example.com/ ", " token ")
	if c.baseURL != "http://example.com" {
		t.Fatalf("baseURL = %q, want http://example.com", c.baseURL)
	}

	if c.logger == nil {
		t.Fatal("logger = nil, want default logger")
	}

	if c.client == nil {
		t.Fatal("client = nil, want initialized http client")
	}
}

func TestH2CClientSendMessage(t *testing.T) {
	var (
		got          ReplyRequest
		gotSignature string
	)

	server := newReplyCaptureServer(t, &got, &gotSignature)
	defer server.Close()

	client := NewH2CClient(server.URL, " bot-token ", WithTransport("http1"))
	if err := client.SendMessage(t.Context(), "room-a", "hello", WithThreadID("12345"), WithThreadScope(2)); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	assertSendMessageRequest(t, gotSignature, got)
}

func TestH2CClientSendMessageValidationError(t *testing.T) {
	client := NewH2CClient("http://example.com", "", WithTransport("http1"))

	err := client.SendMessage(t.Context(), "room-a", "hello", WithThreadID("abc"))
	if err == nil {
		t.Fatal("SendMessage() error = nil, want validation error")
	}

	if !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("SendMessage() error = %q, want thread validation error", err.Error())
	}
}

func TestH2CClientSendImage(t *testing.T) {
	var (
		gotPath      string
		gotMetadata  replyImageMetadata
		gotImageData []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		metadata, images := readMultipartReplyRequest(t, r)
		gotMetadata = metadata
		if len(images) != 1 {
			t.Fatalf("image parts = %d, want 1", len(images))
		}
		gotImageData = images[0]
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{Success: true, Delivery: "async", RequestID: "req-img", Room: "room-b", Type: "image"}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	imgBytes := []byte{0x89, 0x50, 0x4E, 0x47}
	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	resp, err := client.SendImage(t.Context(), "room-b", imgBytes)
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotMetadata.Type != "image" || gotMetadata.Room != "room-b" {
		t.Fatalf("unexpected metadata: %+v", gotMetadata)
	}
	if string(gotImageData) != string(imgBytes) {
		t.Fatalf("image data = %v, want %v", gotImageData, imgBytes)
	}
	if resp == nil || resp.RequestID != "req-img" || resp.Delivery != "async" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	// 이미지 매니페스트 검증
	if len(gotMetadata.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(gotMetadata.Images))
	}
	spec := gotMetadata.Images[0]
	if spec.Index != 0 {
		t.Fatalf("Images[0].Index = %d, want 0", spec.Index)
	}
	if spec.ByteLength != int64(len(imgBytes)) {
		t.Fatalf("Images[0].ByteLength = %d, want %d", spec.ByteLength, len(imgBytes))
	}
	if spec.ContentType != "image/png" {
		t.Fatalf("Images[0].ContentType = %q, want image/png", spec.ContentType)
	}
	if spec.SHA256Hex == "" {
		t.Fatal("Images[0].SHA256Hex is empty")
	}
}

func TestH2CClientSendMultipleImages(t *testing.T) {
	var (
		gotPath     string
		gotMetadata replyImageMetadata
		gotImages   [][]byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		metadata, images := readMultipartReplyRequest(t, r)
		gotMetadata = metadata
		gotImages = images
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{Success: true, Delivery: "queued", RequestID: "req-multi", Room: "room-c", Type: "image_multiple"}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	images := [][]byte{[]byte("img1"), []byte("img2")}
	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	resp, err := client.SendMultipleImages(t.Context(), "room-c", images,
		WithThreadID("999"), WithThreadScope(2))
	if err != nil {
		t.Fatalf("SendMultipleImages() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotMetadata.Type != "image_multiple" || gotMetadata.Room != "room-c" {
		t.Fatalf("unexpected metadata: %+v", gotMetadata)
	}
	if gotMetadata.ThreadID == nil || *gotMetadata.ThreadID != "999" {
		t.Fatalf("ThreadID = %v, want 999", gotMetadata.ThreadID)
	}
	if gotMetadata.ThreadScope == nil || *gotMetadata.ThreadScope != 2 {
		t.Fatalf("ThreadScope = %v, want 2", gotMetadata.ThreadScope)
	}
	if len(gotImages) != 2 || string(gotImages[0]) != "img1" || string(gotImages[1]) != "img2" {
		t.Fatalf("unexpected images: %q", gotImages)
	}
	if resp == nil || resp.RequestID != "req-multi" || resp.Delivery != "queued" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	// 이미지 매니페스트 검증
	if len(gotMetadata.Images) != 2 {
		t.Fatalf("Images length = %d, want 2", len(gotMetadata.Images))
	}
	for i, img := range images {
		spec := gotMetadata.Images[i]
		if spec.Index != i {
			t.Fatalf("Images[%d].Index = %d, want %d", i, spec.Index, i)
		}
		if spec.ByteLength != int64(len(img)) {
			t.Fatalf("Images[%d].ByteLength = %d, want %d", i, spec.ByteLength, len(img))
		}
		if spec.ContentType != "application/octet-stream" {
			t.Fatalf("Images[%d].ContentType = %q, want application/octet-stream", i, spec.ContentType)
		}
		if spec.SHA256Hex == "" {
			t.Fatalf("Images[%d].SHA256Hex is empty", i)
		}
	}
}

func TestSendImageLargePayload(t *testing.T) {
	t.Parallel()

	var (
		receivedMetadata replyImageMetadata
		receivedImage    []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata, images := readMultipartReplyRequest(t, r)
		receivedMetadata = metadata
		if len(images) != 1 {
			t.Fatalf("image parts = %d, want 1", len(images))
		}
		receivedImage = images[0]
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{Success: true, Delivery: "async", RequestID: "req-large", Room: "room", Type: "image"}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	largePayload := bytes.Repeat([]byte("A"), 1<<20)
	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if _, err := client.SendImage(t.Context(), "room", largePayload); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if receivedMetadata.Type != "image" || receivedMetadata.Room != "room" {
		t.Fatalf("unexpected metadata: %+v", receivedMetadata)
	}
	if len(receivedImage) != len(largePayload) {
		t.Fatalf("received size = %d, want %d", len(receivedImage), len(largePayload))
	}

	// 대용량 이미지 매니페스트 검증
	if len(receivedMetadata.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(receivedMetadata.Images))
	}
	spec := receivedMetadata.Images[0]
	if spec.ByteLength != int64(len(largePayload)) {
		t.Fatalf("Images[0].ByteLength = %d, want %d", spec.ByteLength, len(largePayload))
	}
	if spec.ContentType != "application/octet-stream" {
		t.Fatalf("Images[0].ContentType = %q, want application/octet-stream", spec.ContentType)
	}
}

func TestH2CClientGetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		if r.URL.Path != PathConfig {
			t.Fatalf("path = %s, want %s", r.URL.Path, PathConfig)
		}

		resp := ConfigResponse{
			User: ConfigState{
				BotName:                "iris",
				WebEndpoint:            "http://localhost:8080",
				Webhooks:               map[string]string{"default": "http://hook.test"},
				BotHTTPPort:            1234,
				DBPollingRate:          500,
				MessageSendRate:        100,
				CommandRoutePrefixes:   map[string][]string{},
				ImageMessageTypeRoutes: map[string][]string{},
			},
			Applied: ConfigState{
				BotName:                "iris",
				WebEndpoint:            "http://localhost:8080",
				Webhooks:               map[string]string{"default": "http://hook.test"},
				BotHTTPPort:            1234,
				DBPollingRate:          500,
				MessageSendRate:        100,
				CommandRoutePrefixes:   map[string][]string{},
				ImageMessageTypeRoutes: map[string][]string{},
			},
			Discovered: ConfigDiscoveredState{BotID: 7},
			PendingRestart: ConfigPendingRestart{
				Required: false,
				Fields:   []string{},
			},
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode config response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))

	cfg, err := client.GetConfig(t.Context())
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if cfg.User.BotName != "iris" {
		t.Fatalf("User.BotName = %q, want iris", cfg.User.BotName)
	}
	if cfg.User.BotHTTPPort != 1234 {
		t.Fatalf("User.BotHTTPPort = %d, want 1234", cfg.User.BotHTTPPort)
	}
	if cfg.Discovered.BotID != 7 {
		t.Fatalf("Discovered.BotID = %d, want 7", cfg.Discovered.BotID)
	}
	if cfg.User.Webhooks["default"] != "http://hook.test" {
		t.Fatalf("User.Webhooks[default] = %q, want http://hook.test", cfg.User.Webhooks["default"])
	}
}

func TestWithRoundTripperIsUsed(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called.Store(true)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	client := NewH2CClient("http://localhost", "token", WithRoundTripper(rt))
	_ = client.SendMessage(t.Context(), "room", "msg")

	if !called.Load() {
		t.Fatal("custom RoundTripper was not called")
	}
}

func TestPostJSON429Retry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"), WithReplyRetry(3))
	if err := client.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func TestPostJSON429RetryExhausted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"), WithReplyRetry(2))
	err := client.SendMessage(t.Context(), "room", "msg")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
}

func TestPostJSON500NotRetried(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"), WithReplyRetry(3))
	_ = client.SendMessage(t.Context(), "room", "msg")

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (5xx should not retry)", attempts.Load())
	}
}

func TestDoPostJSONUsesReplayableFixedSizeRequestBody(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.ContentLength <= 0 {
			t.Fatalf("ContentLength = %d, want fixed-size request body", r.ContentLength)
		}

		if r.GetBody == nil {
			t.Fatal("GetBody = nil, want replayable request body")
		}

		original, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(request body) error = %v", err)
		}

		replay, err := r.GetBody()
		if err != nil {
			t.Fatalf("GetBody() error = %v", err)
		}
		defer replay.Close()

		replayed, err := io.ReadAll(replay)
		if err != nil {
			t.Fatalf("ReadAll(replayed body) error = %v", err)
		}

		if int64(len(original)) != r.ContentLength {
			t.Fatalf("ContentLength = %d, want %d", r.ContentLength, len(original))
		}

		if string(replayed) != string(original) {
			t.Fatalf("replayed body = %q, want %q", replayed, original)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	client := NewH2CClient("http://localhost", "token", WithRoundTripper(rt))
	if err := client.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type trackingReadCloser struct {
	payload        string
	readBytes      int
	closed         bool
	drainedOnClose bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	if r.readBytes >= len(r.payload) {
		return 0, io.EOF
	}

	n := copy(p, r.payload[r.readBytes:])
	r.readBytes += n
	return n, nil
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	r.drainedOnClose = r.readBytes == len(r.payload)
	return nil
}

func TestH2CClientErrorResponses(t *testing.T) {
	tests := []h2cErrorResponseTestCase{
		{
			name: "send message returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				return c.SendMessage(t.Context(), "room", "msg")
			},
			wantIn: "send iris reply: post /reply: iris /reply returned 500: boom",
		},
		{
			name: "send image returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				_, err := c.SendImage(t.Context(), "room", []byte("img"))
				return err
			},
			wantIn: "send iris image: post /reply: iris /reply returned 500: boom",
		},
		{
			name: "send multiple images returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				_, err := c.SendMultipleImages(t.Context(), "room", [][]byte{[]byte("img")})
				return err
			},
			wantIn: "send iris multiple images: post /reply: iris /reply returned 500: boom",
		},
		{
			name:   "get config returns http error",
			path:   PathConfig,
			call:   getConfigError,
			wantIn: "iris /config returned 500: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runH2CErrorResponseTest(t, tt)
		})
	}
}

func TestClientFailureResponsesAreFullyDrainedBeforeClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		call   func(*testing.T, *H2CClient) error
	}{
		{
			name:   "reply 429 body is drained",
			status: http.StatusTooManyRequests,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				return c.SendMessage(t.Context(), "room", "msg")
			},
		},
		{
			name:   "reply 500 body is drained",
			status: http.StatusInternalServerError,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				return c.SendMessage(t.Context(), "room", "msg")
			},
		},
		{
			name:   "config 500 body is drained",
			status: http.StatusInternalServerError,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				_, err := c.GetConfig(t.Context())
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := &trackingReadCloser{payload: strings.Repeat("x", 16<<10)}
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Body:       body,
					Header:     make(http.Header),
				}, nil
			})

			client := NewH2CClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(1))
			err := tt.call(t, client)
			if err == nil {
				t.Fatal("error = nil, want failure")
			}

			if !body.closed {
				t.Fatal("response body was not closed")
			}

			if !body.drainedOnClose {
				t.Fatalf("response body closed before drain: read=%d total=%d", body.readBytes, len(body.payload))
			}
		})
	}
}

type h2cErrorResponseTestCase struct {
	name   string
	path   string
	call   func(*testing.T, *H2CClient) error
	wantIn string
}

func newReplyCaptureServer(t *testing.T, got *ReplyRequest, gotSignature *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequestMethodAndPath(t, r, http.MethodPost, PathReply)

		*gotSignature = r.Header.Get(HeaderIrisSignature)

		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
}

func readMultipartReplyRequest(t *testing.T, r *http.Request) (replyImageMetadata, [][]byte) {
	t.Helper()

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}

	mr, err := r.MultipartReader()
	if err != nil {
		t.Fatalf("MultipartReader() error = %v", err)
	}

	var (
		metadata   replyImageMetadata
		seenMeta   bool
		imageParts [][]byte
	)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}

		payload, err := io.ReadAll(part)
		if err != nil {
			part.Close()
			t.Fatalf("ReadAll(part) error = %v", err)
		}
		if err := part.Close(); err != nil {
			t.Fatalf("part.Close() error = %v", err)
		}

		switch part.FormName() {
		case "metadata":
			if seenMeta {
				t.Fatal("metadata part duplicated")
			}
			if err := jsonx.Unmarshal(payload, &metadata); err != nil {
				t.Fatalf("jsonx.Unmarshal(metadata) error = %v", err)
			}
			seenMeta = true
		case "image":
			imageParts = append(imageParts, payload)
		default:
			t.Fatalf("unexpected form part %q", part.FormName())
		}
	}

	if !seenMeta {
		t.Fatal("metadata part missing")
	}

	return metadata, imageParts
}

func assertRequestMethodAndPath(t *testing.T, r *http.Request, wantMethod, wantPath string) {
	t.Helper()

	if r.Method != wantMethod {
		t.Fatalf("method = %s, want %s", r.Method, wantMethod)
	}

	if r.URL.Path != wantPath {
		t.Fatalf("path = %s, want %s", r.URL.Path, wantPath)
	}
}

func assertSendMessageRequest(t *testing.T, gotSignature string, got ReplyRequest) {
	t.Helper()

	if gotSignature == "" {
		t.Fatal("signature header missing")
	}

	if got.Type != "text" || got.Room != "room-a" || got.Data != "hello" {
		t.Fatalf("unexpected request body: %+v", got)
	}

	if got.ThreadID == nil || *got.ThreadID != "12345" {
		t.Fatalf("ThreadID = %v, want 12345", got.ThreadID)
	}

	if got.ThreadScope == nil || *got.ThreadScope != 2 {
		t.Fatalf("ThreadScope = %v, want 2", got.ThreadScope)
	}
}

func runH2CErrorResponseTest(t *testing.T, tt h2cErrorResponseTestCase) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tt.path {
			t.Fatalf("path = %s, want %s", r.URL.Path, tt.path)
		}

		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))

	err := tt.call(t, client)
	if err == nil {
		t.Fatal("error = nil, want failure")
	}

	if !strings.Contains(err.Error(), tt.wantIn) {
		t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantIn)
	}
}

func getConfigError(t *testing.T, c *H2CClient) error {
	t.Helper()

	_, err := c.GetConfig(t.Context())
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}

	return nil
}

func TestWithHTTPClientTakesPrecedenceOverRoundTripper(t *testing.T) {
	t.Parallel()

	var httpClientCalled atomic.Bool
	customClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			httpClientCalled.Store(true)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	var rtCalled atomic.Bool
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rtCalled.Store(true)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	client := NewH2CClient("http://localhost", "token",
		WithRoundTripper(rt),
		WithHTTPClient(customClient),
	)
	_ = client.SendMessage(t.Context(), "room", "msg")

	if !httpClientCalled.Load() {
		t.Fatal("WithHTTPClient should take precedence")
	}
	if rtCalled.Load() {
		t.Fatal("WithRoundTripper should not be used when WithHTTPClient is set")
	}
}

func TestQueryRoomSummary429NotRetried(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"), WithReplyRetry(3))
	_, _ = client.QueryRoomSummary(t.Context(), 42)

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (non-reply paths should not retry)", attempts.Load())
	}
}

func TestH2CClientSendMarkdown(t *testing.T) {
	t.Parallel()

	var (
		gotPath string
		gotBody ReplyRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		resp := ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "async",
			RequestID: "req-123",
			Room:      "room-a",
			Type:      "text",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.SendMarkdown(t.Context(), "room-a", "# Hello", WithThreadID("12345"), WithThreadScope(2))
	if err != nil {
		t.Fatalf("SendMarkdown() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotBody.Type != "markdown" || gotBody.Room != "room-a" || gotBody.Data != "# Hello" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}

	if gotBody.ThreadID == nil || *gotBody.ThreadID != "12345" {
		t.Fatalf("ThreadID = %v, want 12345", gotBody.ThreadID)
	}

	if !result.Success || result.RequestID != "req-123" || result.Delivery != "async" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestH2CClientSendMarkdownValidationError(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://example.com", "", WithTransport("http1"))
	_, err := client.SendMarkdown(t.Context(), "room", "md", WithThreadID("abc"))
	if err == nil {
		t.Fatal("SendMarkdown() error = nil, want validation error")
	}

	if !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("error = %q, want thread validation error", err.Error())
	}
}

func TestH2CClientGetReplyStatus(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		detail := "delivered"
		resp := ReplyStatusSnapshot{
			RequestID:        "req-abc",
			State:            "completed",
			UpdatedAtEpochMs: 1711600000000,
			Detail:           &detail,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	snap, err := client.GetReplyStatus(t.Context(), "req-abc")
	if err != nil {
		t.Fatalf("GetReplyStatus() error = %v", err)
	}

	if gotPath != "/reply-status/req-abc" {
		t.Fatalf("path = %q, want /reply-status/req-abc", gotPath)
	}

	if snap.RequestID != "req-abc" || snap.State != "completed" || snap.UpdatedAtEpochMs != 1711600000000 {
		t.Fatalf("unexpected response: %+v", snap)
	}

	if snap.Detail == nil || *snap.Detail != "delivered" {
		t.Fatalf("Detail = %v, want delivered", snap.Detail)
	}
}

func TestH2CClientGetReplyStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.GetReplyStatus(t.Context(), "missing")
	if err == nil {
		t.Fatal("expected error for 404")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %q, want 404 mention", err.Error())
	}
}

func TestH2CClientUpdateConfig(t *testing.T) {
	t.Parallel()

	var (
		gotPath string
		gotBody ConfigUpdateRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		resp := ConfigUpdateResponse{
			Success:   true,
			Name:      "endpoint",
			Persisted: true,
			Applied:   true,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	endpoint := "http://new.endpoint"
	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.UpdateConfig(t.Context(), "endpoint", ConfigUpdateRequest{
		Endpoint: &endpoint,
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	if gotPath != "/config/endpoint" {
		t.Fatalf("path = %q, want /config/endpoint", gotPath)
	}

	if gotBody.Endpoint == nil || *gotBody.Endpoint != "http://new.endpoint" {
		t.Fatalf("request endpoint = %v, want http://new.endpoint", gotBody.Endpoint)
	}

	if !result.Success || result.Name != "endpoint" || !result.Persisted {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestH2CClientUpdateConfigError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.UpdateConfig(t.Context(), "bogus", ConfigUpdateRequest{})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestH2CClientGetBridgeHealth(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := BridgeHealthResult{
			Reachable:    true,
			Running:      true,
			SpecReady:    true,
			RestartCount: 0,
			Checks: []BridgeHealthCheck{
				{Name: "connectivity", OK: true},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetBridgeHealth(t.Context())
	if err != nil {
		t.Fatalf("GetBridgeHealth() error = %v", err)
	}

	if gotPath != PathDiagnosticsBridge {
		t.Fatalf("path = %q, want %q", gotPath, PathDiagnosticsBridge)
	}

	if !result.Reachable || !result.Running || !result.SpecReady {
		t.Fatalf("unexpected result: %+v", result)
	}

	if len(result.Checks) != 1 || result.Checks[0].Name != "connectivity" || !result.Checks[0].OK {
		t.Fatalf("unexpected checks: %+v", result.Checks)
	}
}

func TestH2CClientGetBridgeHealthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.GetBridgeHealth(t.Context())
	if err == nil {
		t.Fatal("expected error for 503")
	}

	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %q, want 503 mention", err.Error())
	}
}

func TestDoPostJSONPipeCleanupOnTransportError(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("connection refused")
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	client := NewH2CClient("http://localhost", "token", WithRoundTripper(rt))

	before := runtime.NumGoroutine()
	for range 10 {
		_ = client.SendMessage(t.Context(), "room", "msg")
	}

	// encoder goroutine이 해제될 시간
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	// goroutine 누수가 없으면 차이가 작아야 함
	if after-before > 5 {
		t.Fatalf("goroutine leak: before=%d, after=%d (diff=%d)", before, after, after-before)
	}
}

func TestH2CClientSplitAuthUsesInboundSecretForConfig(t *testing.T) {
	t.Parallel()

	// inbound/botControl 비밀키가 모두 설정된 경우
	// GET /config는 inbound 비밀키로 서명해야 함
	inboundSecret := "inbound-secret-abc"
	botControlSecret := "bot-control-xyz"

	var capturedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSig = r.Header.Get("X-Iris-Signature")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":{}}`))
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "unused-bot-token",
		WithInboundSecret(inboundSecret),
		WithBotControlToken(botControlSecret),
		WithHTTPClient(srv.Client()),
	)

	_, err := c.GetConfig(t.Context())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if capturedSig == "" {
		t.Fatal("expected signature header to be set")
	}
}

func TestH2CClientSplitAuthUsesBotControlForReply(t *testing.T) {
	t.Parallel()

	inboundSecret := "inbound-secret-abc"
	botControlSecret := "bot-control-xyz"

	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "unused-bot-token",
		WithInboundSecret(inboundSecret),
		WithBotControlToken(botControlSecret),
		WithHTTPClient(srv.Client()),
	)

	err := c.SendMessage(t.Context(), "room1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if capturedHeaders.Get("X-Iris-Signature") == "" {
		t.Fatal("expected signature header")
	}
}

func TestH2CClientSplitAuthVerifiesCorrectSecret(t *testing.T) {
	t.Parallel()

	// split secret 설정 시 라우트 카테고리별 올바른 비밀키로 HMAC을 계산하는지 검증
	inboundSecret := "inbound-123"
	botControlSecret := "botctl-456"

	signatures := make(map[string]string) // path -> signature

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatures[r.URL.Path] = r.Header.Get("X-Iris-Signature")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/config":
			w.Write([]byte(`{"state":{}}`))
		case "/rooms":
			w.Write([]byte(`{"rooms":[]}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "unused",
		WithInboundSecret(inboundSecret),
		WithBotControlToken(botControlSecret),
		WithHTTPClient(srv.Client()),
	)

	c.GetConfig(t.Context())
	c.SendMessage(t.Context(), "r", "msg")
	c.GetRooms(t.Context())

	// config와 reply는 서로 다른 비밀키를 사용하므로 서명이 달라야 함
	if signatures["/config"] == signatures["/reply"] {
		t.Error("config and reply should use different secrets but produced same signature")
	}

	for path, sig := range signatures {
		if sig == "" {
			t.Errorf("missing signature for %s", path)
		}
	}
}

func TestH2CClientSharedSecretFallback(t *testing.T) {
	t.Parallel()

	// WithHMACSecret(shared)만 설정된 경우 모든 라우트가 shared secret를 사용해야 함
	sharedSecret := "shared-secret"

	sigs := make(map[string]string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigs[r.URL.Path] = r.Header.Get("X-Iris-Signature")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/config" {
			w.Write([]byte(`{"state":{}}`))
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "bot-token",
		WithHMACSecret(sharedSecret),
		WithHTTPClient(srv.Client()),
	)

	c.GetConfig(t.Context())
	c.SendMessage(t.Context(), "r", "msg")

	for path, sig := range sigs {
		if sig == "" {
			t.Errorf("expected signature for %s with shared secret", path)
		}
	}
}

func TestH2CClientBotTokenAsDefaultSharedSecret(t *testing.T) {
	t.Parallel()

	// 명시적 비밀키가 설정되지 않은 경우 botToken이 모든 라우트의 shared secret로 사용되어야 함
	botToken := "my-bot-token"

	var configSig, replySig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Iris-Signature")
		if r.URL.Path == "/config" {
			configSig = sig
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"state":{}}`))
		} else {
			replySig = sig
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, botToken, WithHTTPClient(srv.Client()))

	c.GetConfig(t.Context())
	c.SendMessage(t.Context(), "r", "msg")

	if configSig == "" || replySig == "" {
		t.Fatal("both routes should be signed with bot token as default")
	}
}

func TestPostMultipartBodyIsNotFullyBuffered(t *testing.T) {
	t.Parallel()

	// multipart body가 io.Pipe로 스트리밍되는지 검증:
	// RoundTrip에서 request body 타입이 *io.PipeReader여야 함
	var bodyTypeName string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		bodyTypeName = fmt.Sprintf("%T", r.Body)
		// body를 완전히 읽어야 pipe writer goroutine이 종료됨
		io.Copy(io.Discard, r.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"delivery":"queued","requestId":"r1","room":"room","type":"image"}`)),
		}, nil
	})

	c := NewH2CClient("http://localhost", "tok", WithRoundTripper(rt))
	img := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}
	_, err := c.SendImage(t.Context(), "room", img)
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	// *io.PipeReader 타입이어야 전체 버퍼링이 아닌 스트리밍 방식
	if bodyTypeName != "*io.PipeReader" {
		t.Fatalf("request body type = %s, want *io.PipeReader (streaming)", bodyTypeName)
	}
}

func TestPostMultipart429RetryRegeneratesBody(t *testing.T) {
	t.Parallel()

	// 429 후 재시도 시 multipart body가 정상적으로 재생성되는지 검증
	var attempts atomic.Int32
	var lastBodyLen int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll(body) error on attempt %d: %v", attempts.Load(), err)
		}
		if len(body) == 0 {
			t.Errorf("empty body on attempt %d", attempts.Load())
		}
		lastBodyLen = len(body)

		if attempts.Load() == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "r1",
			Room:      "room",
			Type:      "image",
		})
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "tok", WithHTTPClient(server.Client()), WithReplyRetry(3))
	img := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}
	_, err := c.SendImage(t.Context(), "room", img)
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	if lastBodyLen == 0 {
		t.Fatal("last attempt had empty body")
	}
}
