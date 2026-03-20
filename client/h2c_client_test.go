package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	if got.Type != "image" || got.Room != "room-b" || got.Data != "b64data" {
		t.Fatalf("unexpected request body: %+v", got)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
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
