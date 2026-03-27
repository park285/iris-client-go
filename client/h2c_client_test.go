package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
		got      ReplyRequest
		gotToken string
	)

	server := newReplyCaptureServer(t, &got, &gotToken)
	defer server.Close()

	client := NewH2CClient(server.URL, " bot-token ", WithTransport("http1"))
	if err := client.SendMessage(t.Context(), "room-a", "hello", WithThreadID("12345"), WithThreadScope(2)); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	assertSendMessageRequest(t, gotToken, got)
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
	var got ReplyRequest
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if err := client.SendImage(t.Context(), "room-b", "b64data"); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if gotPath != PathReplyImage {
		t.Fatalf("path = %q, want %q", gotPath, PathReplyImage)
	}

	if got.Type != "image" || got.Room != "room-b" || got.Data != "b64data" {
		t.Fatalf("unexpected request body: %+v", got)
	}
}

func TestH2CClientSendMultipleImages(t *testing.T) {
	var gotPath string
	var gotBody json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if err := client.SendMultipleImages(t.Context(), "room-c", []string{"img1", "img2"},
		WithThreadID("999"), WithThreadScope(2)); err != nil {
		t.Fatalf("SendMultipleImages() error = %v", err)
	}

	if gotPath != PathReplyImage {
		t.Fatalf("path = %q, want %q", gotPath, PathReplyImage)
	}

	var parsed struct {
		Type        string   `json:"type"`
		Room        string   `json:"room"`
		Data        []string `json:"data"`
		ThreadID    *string  `json:"threadId"`
		ThreadScope *int     `json:"threadScope"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if parsed.Type != "image_multiple" || parsed.Room != "room-c" {
		t.Fatalf("unexpected type/room: %+v", parsed)
	}
	if len(parsed.Data) != 2 || parsed.Data[0] != "img1" || parsed.Data[1] != "img2" {
		t.Fatalf("unexpected data: %v", parsed.Data)
	}
	if parsed.ThreadID == nil || *parsed.ThreadID != "999" {
		t.Fatalf("ThreadID = %v, want 999", parsed.ThreadID)
	}
	if parsed.ThreadScope == nil || *parsed.ThreadScope != 2 {
		t.Fatalf("ThreadScope = %v, want 2", parsed.ThreadScope)
	}
}

func TestSendImageLargePayloadStreams(t *testing.T) {
	t.Parallel()

	var receivedSize int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		receivedSize = len(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	largePayload := strings.Repeat("A", 1<<20)
	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if err := client.SendImage(t.Context(), "room", largePayload); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if receivedSize == 0 {
		t.Fatal("server received empty body")
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

		if err := json.NewEncoder(w).Encode(Config{BotName: "iris", BotHTTPPort: 1234, DBPollingRate: 5, MessageSendRate: 6, BotID: 7}); err != nil {
			t.Fatalf("encode config response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))

	cfg, err := client.GetConfig(t.Context())
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if cfg.BotName != "iris" || cfg.BotHTTPPort != 1234 || cfg.BotID != 7 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestH2CClientDecrypt(t *testing.T) {
	var got DecryptRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathDecrypt {
			t.Fatalf("path = %s, want %s", r.URL.Path, PathDecrypt)
		}

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if err := json.NewEncoder(w).Encode(DecryptResponse{PlainText: "plain"}); err != nil {
			t.Fatalf("encode decrypt response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))

	plaintext, err := client.Decrypt(t.Context(), "cipher")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if plaintext != "plain" {
		t.Fatalf("Decrypt() = %q, want plain", plaintext)
	}

	if got.B64Ciphertext != "cipher" || got.Enc != 0 {
		t.Fatalf("unexpected decrypt request: %+v", got)
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
			path: PathReplyImage,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				return c.SendImage(t.Context(), "room", "b64")
			},
			wantIn: "send iris image: post /reply-image: iris /reply-image returned 500: boom",
		},
		{
			name: "send multiple images returns wrapped error",
			path: PathReplyImage,
			call: func(t *testing.T, c *H2CClient) error {
				t.Helper()
				return c.SendMultipleImages(t.Context(), "room", []string{"b64"})
			},
			wantIn: "send iris multiple images: post /reply-image: iris /reply-image returned 500: boom",
		},
		{
			name:   "get config returns http error",
			path:   PathConfig,
			call:   getConfigError,
			wantIn: "iris /config returned 500: boom",
		},
		{
			name:   "decrypt returns http error",
			path:   PathDecrypt,
			call:   decryptError,
			wantIn: "iris /decrypt returned 500: boom",
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

func newReplyCaptureServer(t *testing.T, got *ReplyRequest, gotToken *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequestMethodAndPath(t, r, http.MethodPost, PathReply)

		*gotToken = r.Header.Get(HeaderBotToken)

		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
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

func assertSendMessageRequest(t *testing.T, gotToken string, got ReplyRequest) {
	t.Helper()

	if gotToken != "bot-token" {
		t.Fatalf("bot token header = %q, want bot-token", gotToken)
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

func decryptError(t *testing.T, c *H2CClient) error {
	t.Helper()

	_, err := c.Decrypt(t.Context(), "cipher")
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
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

func TestDecrypt429NotRetried(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"), WithReplyRetry(3))
	_, _ = client.Decrypt(t.Context(), "cipher")

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (non-reply paths should not retry)", attempts.Load())
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
