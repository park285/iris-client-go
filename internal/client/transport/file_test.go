package transport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSendFileUsesIrisMultipartContract(t *testing.T) {
	t.Parallel()

	payload := []byte("quarterly-report")
	var got struct {
		contentLength int64
		metadata      commonReplyFileMetadataForTest
		formName      string
		fileName      string
		contentType   string
		payload       []byte
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.contentLength = r.ContentLength
		if len(r.TransferEncoding) != 0 {
			t.Errorf("TransferEncoding = %v, want known-length request", r.TransferEncoding)
		}

		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader() error = %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}

		metadataPart, err := reader.NextPart()
		if err != nil {
			t.Errorf("NextPart(metadata) error = %v", err)
			http.Error(w, "missing metadata", http.StatusBadRequest)
			return
		}
		if metadataPart.FormName() != "metadata" {
			t.Errorf("metadata form name = %q", metadataPart.FormName())
		}
		if err := json.NewDecoder(metadataPart).Decode(&got.metadata); err != nil {
			t.Errorf("decode metadata: %v", err)
			http.Error(w, "bad metadata", http.StatusBadRequest)
			return
		}

		filePart, err := reader.NextPart()
		if err != nil {
			t.Errorf("NextPart(file) error = %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		got.formName = filePart.FormName()
		got.fileName = filePart.FileName()
		got.contentType = filePart.Header.Get("Content-Type")
		got.payload, err = io.ReadAll(filePart)
		if err != nil {
			t.Errorf("ReadAll(file) error = %v", err)
			http.Error(w, "bad file", http.StatusBadRequest)
			return
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Errorf("NextPart(after file) error = %v, want EOF", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "reply-file-1",
			Room:      "room-a",
			Type:      msgTypeFile,
		}); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithHTTPClient(server.Client()))
	accepted, err := client.SendFile(
		t.Context(),
		"room-a",
		NewReplyFileBytes("분기 보고서 final.PDF", "Application/PDF", payload),
		WithClientRequestID("client:file:0001"),
		WithThreadID("456"),
		WithThreadScope(2),
	)
	if err != nil {
		t.Fatalf("SendFile() error = %v", err)
	}
	if accepted == nil || accepted.Type != msgTypeFile {
		t.Fatalf("accepted = %+v, want file response", accepted)
	}
	if got.contentLength <= 0 {
		t.Fatalf("ContentLength = %d, want positive", got.contentLength)
	}
	if got.metadata.ClientRequestID != "client:file:0001" || got.metadata.Type != msgTypeFile || got.metadata.Room != "room-a" {
		t.Fatalf("metadata = %+v", got.metadata)
	}
	if got.metadata.ThreadID != "456" || got.metadata.ThreadScope != 2 {
		t.Fatalf("thread metadata = id %q scope %d", got.metadata.ThreadID, got.metadata.ThreadScope)
	}
	if len(got.metadata.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(got.metadata.Files))
	}
	manifest := got.metadata.Files[0]
	fileHash := sha256.Sum256(payload)
	if manifest.Index != 0 || manifest.SHA256Hex != hex.EncodeToString(fileHash[:]) || manifest.ByteLength != int64(len(payload)) {
		t.Fatalf("file manifest = %+v", manifest)
	}
	if manifest.ContentType != "application/pdf" || manifest.FileName != "분기 보고서 final.PDF" {
		t.Fatalf("file manifest identity = %+v", manifest)
	}
	if got.formName != "file" || got.fileName != manifest.FileName || got.contentType != manifest.ContentType {
		t.Fatalf("file part = form %q name %q type %q", got.formName, got.fileName, got.contentType)
	}
	if !bytes.Equal(got.payload, payload) {
		t.Fatalf("payload = %q, want %q", got.payload, payload)
	}
}

func TestSendFileRejectsUnsafeNameBeforeNetwork(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://127.0.0.1:1", "", WithTransport("http1"))
	_, err := client.SendFile(t.Context(), "room", NewReplyFileBytes("../secret.txt", "text/plain", []byte("x")))
	if err == nil || !strings.Contains(err.Error(), "invalid file name") {
		t.Fatalf("SendFile() error = %v, want invalid file name", err)
	}
}

func TestSendFilePathInfersMIMEAndClosesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("report"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader() error = %v", err)
			return
		}
		_, _ = reader.NextPart()
		filePart, err := reader.NextPart()
		if err != nil {
			t.Errorf("NextPart(file) error = %v", err)
			return
		}
		gotContentType = filePart.Header.Get("Content-Type")
		_, _ = io.Copy(io.Discard, filePart)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"delivery":"sent","requestId":"path-file","room":"room","type":"file"}`)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithHTTPClient(server.Client()))
	if _, err := client.SendFilePath(t.Context(), "room", path, ""); err != nil {
		t.Fatalf("SendFilePath() error = %v", err)
	}
	if gotContentType != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", gotContentType)
	}

	// The helper owns and closes its descriptor. This rename also catches a
	// leaked handle on platforms that prevent renaming open files.
	renamed := path + ".done"
	if err := os.Rename(path, renamed); err != nil {
		t.Fatalf("Rename() after SendFilePath error = %v", err)
	}
}

type commonReplyFileMetadataForTest struct {
	ClientRequestID string `json:"clientRequestId"`
	Type            string `json:"type"`
	Room            string `json:"room"`
	ThreadID        string `json:"threadId"`
	ThreadScope     int    `json:"threadScope"`
	Files           []struct {
		Index       int    `json:"index"`
		SHA256Hex   string `json:"sha256Hex"`
		ByteLength  int64  `json:"byteLength"`
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
	} `json:"files"`
}

func TestSendFileTransportRetryRequiresClientRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		clientRequestID string
		wantAttempts    int32
		wantError       bool
	}{
		{name: "without idempotency key", wantAttempts: 1, wantError: true},
		{name: "with idempotency key", clientRequestID: "client:file:0001", wantAttempts: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			var bodiesMu sync.Mutex
			var bodies [][]byte
			roundTripper := fileRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, fmt.Errorf("read request body: %w", err)
				}
				if err := req.Body.Close(); err != nil {
					return nil, fmt.Errorf("close request body: %w", err)
				}
				bodiesMu.Lock()
				bodies = append(bodies, body)
				bodiesMu.Unlock()

				if attempts.Add(1) == 1 {
					return nil, errors.New("connection reset")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"success":true,"delivery":"sent","requestId":"retry-file","room":"room","type":"file"}`)),
				}, nil
			})

			client := NewH2CClient(
				"http://iris.test",
				"",
				WithRoundTripper(roundTripper),
				WithReplyRetry(2),
			)
			var opts []SendOption
			if test.clientRequestID != "" {
				opts = append(opts, WithClientRequestID(test.clientRequestID))
			}

			_, err := client.SendFile(
				t.Context(),
				"room",
				NewReplyFileBytes("payload.bin", "application/octet-stream", []byte("payload")),
				opts...,
			)
			if test.wantError && err == nil {
				t.Fatal("SendFile() error = nil, want transport error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("SendFile() error = %v", err)
			}
			if got := attempts.Load(); got != test.wantAttempts {
				t.Fatalf("attempts = %d, want %d", got, test.wantAttempts)
			}
			if len(bodies) == 2 && !bytes.Equal(bodies[0], bodies[1]) {
				t.Fatal("retry multipart bodies differ")
			}
		})
	}
}

type fileRoundTripFunc func(*http.Request) (*http.Response, error)

func (f fileRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
