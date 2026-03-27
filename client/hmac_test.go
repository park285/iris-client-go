package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignIrisRequest(t *testing.T) {
	t.Parallel()

	sig1 := signIrisRequest("secret-a", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)
	sig2 := signIrisRequest("secret-b", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)

	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}

	if len(sig1) != 64 {
		t.Fatalf("signature length = %d, want 64 hex chars", len(sig1))
	}

	// Deterministic: same inputs produce same output.
	sig1Again := signIrisRequest("secret-a", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)
	if sig1 != sig1Again {
		t.Fatalf("same inputs produced different sigs: %q vs %q", sig1, sig1Again)
	}
}

func TestSignIrisRequestEmptyBody(t *testing.T) {
	t.Parallel()

	sig := signIrisRequest("secret", "GET", "/config", "1711600000000", "nonce1", "")
	if sig == "" {
		t.Fatal("signature should not be empty for empty body")
	}

	if len(sig) != 64 {
		t.Fatalf("signature length = %d, want 64 hex chars", len(sig))
	}
}

func TestSignIrisRequestMethodCaseInsensitive(t *testing.T) {
	t.Parallel()

	sig1 := signIrisRequest("secret", "get", "/config", "123", "n", "")
	sig2 := signIrisRequest("secret", "GET", "/config", "123", "n", "")

	if sig1 != sig2 {
		t.Fatal("method should be case-insensitive (uppercased in canonical form)")
	}
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	n1 := generateNonce()
	n2 := generateNonce()

	if n1 == n2 {
		t.Fatal("two consecutive nonces should differ")
	}

	if len(n1) != 32 {
		t.Fatalf("nonce length = %d, want 32 hex chars (16 bytes)", len(n1))
	}
}

func TestH2CClientHMACHeaders(t *testing.T) {
	t.Parallel()

	var (
		gotTimestamp string
		gotNonce     string
		gotSignature string
		gotBotToken  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotNonce = r.Header.Get(HeaderIrisNonce)
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotBotToken = r.Header.Get(HeaderBotToken)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "my-token",
		WithTransport("http1"),
		WithHMACSecret("test-secret"),
	)

	if err := c.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotTimestamp == "" {
		t.Fatal("X-Iris-Timestamp header missing")
	}

	if gotNonce == "" {
		t.Fatal("X-Iris-Nonce header missing")
	}

	if gotSignature == "" {
		t.Fatal("X-Iris-Signature header missing")
	}

	if len(gotSignature) != 64 {
		t.Fatalf("signature length = %d, want 64", len(gotSignature))
	}

	// When HMAC is active, X-Bot-Token should NOT be sent.
	if gotBotToken != "" {
		t.Fatalf("X-Bot-Token = %q, want empty when HMAC is active", gotBotToken)
	}
}

func TestH2CClientHMACHeadersOnGET(t *testing.T) {
	t.Parallel()

	var (
		gotTimestamp string
		gotSignature string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotSignature = r.Header.Get(HeaderIrisSignature)

		resp := ConfigResponse{
			User:    ConfigState{BotName: "iris"},
			Applied: ConfigState{BotName: "iris"},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "my-token",
		WithTransport("http1"),
		WithHMACSecret("test-secret"),
	)

	if _, err := c.GetConfig(t.Context()); err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if gotTimestamp == "" {
		t.Fatal("X-Iris-Timestamp header missing on GET")
	}

	if gotSignature == "" {
		t.Fatal("X-Iris-Signature header missing on GET")
	}
}

func TestH2CClientPlainTokenWhenNoHMAC(t *testing.T) {
	t.Parallel()

	var (
		gotBotToken  string
		gotTimestamp string
		gotSignature string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBotToken = r.Header.Get(HeaderBotToken)
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotSignature = r.Header.Get(HeaderIrisSignature)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "plain-token", WithTransport("http1"))

	if err := c.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotBotToken != "plain-token" {
		t.Fatalf("X-Bot-Token = %q, want plain-token", gotBotToken)
	}

	if gotTimestamp != "" {
		t.Fatalf("X-Iris-Timestamp = %q, want empty when no HMAC", gotTimestamp)
	}

	if gotSignature != "" {
		t.Fatalf("X-Iris-Signature = %q, want empty when no HMAC", gotSignature)
	}
}

func TestH2CClientPlainTokenOnGETWhenNoHMAC(t *testing.T) {
	t.Parallel()

	var gotBotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBotToken = r.Header.Get(HeaderBotToken)
		resp := ConfigResponse{
			User:    ConfigState{BotName: "iris"},
			Applied: ConfigState{BotName: "iris"},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "my-token", WithTransport("http1"))

	if _, err := c.GetConfig(t.Context()); err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if gotBotToken != "my-token" {
		t.Fatalf("X-Bot-Token = %q, want my-token", gotBotToken)
	}
}

func TestH2CClientHMACSignatureVerifiable(t *testing.T) {
	t.Parallel()

	const hmacSecret = "verify-secret"

	var (
		capturedTimestamp string
		capturedNonce     string
		capturedSignature string
		capturedBody      string
		capturedMethod    string
		capturedPath      string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedTimestamp = r.Header.Get(HeaderIrisTimestamp)
		capturedNonce = r.Header.Get(HeaderIrisNonce)
		capturedSignature = r.Header.Get(HeaderIrisSignature)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = string(body)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "token",
		WithTransport("http1"),
		WithHMACSecret(hmacSecret),
	)

	if err := c.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	// Re-compute the expected signature from captured values.
	expected := signIrisRequest(
		hmacSecret,
		capturedMethod,
		capturedPath,
		capturedTimestamp,
		capturedNonce,
		capturedBody,
	)

	if capturedSignature != expected {
		t.Fatalf("signature mismatch:\n  got:  %s\n  want: %s", capturedSignature, expected)
	}
}

func TestH2CClientNoAuthHeadersWhenBothEmpty(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get(HeaderBotToken); got != "" {
			t.Fatalf("X-Bot-Token = %q, want empty", got)
		}
		if got := r.Header.Get(HeaderIrisSignature); got != "" {
			t.Fatalf("X-Iris-Signature = %q, want empty", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	c := NewH2CClient("http://localhost", "", WithRoundTripper(rt))
	_ = c.SendMessage(t.Context(), "room", "msg")
}
