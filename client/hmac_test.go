package client

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
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
		gotBodyHash  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotNonce = r.Header.Get(HeaderIrisNonce)
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotBodyHash = r.Header.Get(HeaderIrisBodySHA256)
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
	if gotBodyHash == "" {
		t.Fatal("X-Iris-Body-Sha256 header missing")
	}

	if len(gotSignature) != 64 {
		t.Fatalf("signature length = %d, want 64", len(gotSignature))
	}
	if len(gotBodyHash) != 64 {
		t.Fatalf("body hash length = %d, want 64", len(gotBodyHash))
	}

}

func TestH2CClientHMACHeadersOnGET(t *testing.T) {
	t.Parallel()

	var (
		gotTimestamp string
		gotSignature string
		gotBodyHash  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotBodyHash = r.Header.Get(HeaderIrisBodySHA256)

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
	if gotBodyHash == "" {
		t.Fatal("X-Iris-Body-Sha256 header missing on GET")
	}
}

func TestH2CClientBotTokenSignsWhenNoHMAC(t *testing.T) {
	t.Parallel()

	var (
		gotTimestamp string
		gotSignature string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotSignature = r.Header.Get(HeaderIrisSignature)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "plain-token", WithTransport("http1"))

	if err := c.SendMessage(t.Context(), "room", "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotTimestamp == "" {
		t.Fatal("X-Iris-Timestamp header missing when signing with bot token")
	}

	if gotSignature == "" {
		t.Fatal("X-Iris-Signature header missing when signing with bot token")
	}
}

func TestH2CClientBotTokenSignsGETWhenNoHMAC(t *testing.T) {
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

	c := NewH2CClient(server.URL, "my-token", WithTransport("http1"))

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

func TestH2CClientMultipartHMACSignsMetadataOnly(t *testing.T) {
	t.Parallel()

	const hmacSecret = "verify-secret"

	var (
		capturedTimestamp string
		capturedNonce     string
		capturedSignature string
		capturedMethod    string
		capturedPath      string
		capturedMetadata  string
		capturedBody      string
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

		if err := r.Body.Close(); err != nil {
			t.Fatalf("body.Close() error = %v", err)
		}
		r.Body = io.NopCloser(strings.NewReader(capturedBody))

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
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}
			payload, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll(part) error = %v", err)
			}
			if part.FormName() == "metadata" {
				capturedMetadata = string(payload)
			}
			part.Close()
		}

		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{Success: true, Delivery: "async", RequestID: "req-hmac", Room: "room", Type: "image"}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "token",
		WithTransport("http1"),
		WithHMACSecret(hmacSecret),
	)

	if _, err := c.SendImage(t.Context(), "room", []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if capturedMetadata == "" {
		t.Fatal("metadata part missing")
	}

	expected := signIrisRequest(
		hmacSecret,
		capturedMethod,
		capturedPath,
		capturedTimestamp,
		capturedNonce,
		capturedMetadata,
	)

	if capturedSignature != expected {
		t.Fatalf("signature mismatch:\n  got:  %s\n  want: %s", capturedSignature, expected)
	}

	if capturedSignature == signIrisRequest(
		hmacSecret,
		capturedMethod,
		capturedPath,
		capturedTimestamp,
		capturedNonce,
		capturedBody,
	) {
		t.Fatal("multipart signature should not be computed from full body")
	}

	var metadata replyImageMetadata
	if err := jsonx.Unmarshal([]byte(capturedMetadata), &metadata); err != nil {
		t.Fatalf("jsonx.Unmarshal(metadata) error = %v", err)
	}
	if metadata.Type != "image" || metadata.Room != "room" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func TestH2CClientNoAuthHeadersWhenBothEmpty(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
