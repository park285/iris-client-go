package transport

import (
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

func TestSignIrisRequest(t *testing.T) {
	t.Parallel()

	sig1 := mustSignIrisRequest(t, "secret-a", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)
	sig2 := mustSignIrisRequest(t, "secret-b", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)

	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}

	if len(sig1) != 64 {
		t.Fatalf("signature length = %d, want 64 hex chars", len(sig1))
	}

	// 결정론적: 동일 입력은 동일 출력을 낸다.
	sig1Again := mustSignIrisRequest(t, "secret-a", "POST", "/reply", "1711600000000", "abc123", `{"room":"r"}`)
	if sig1 != sig1Again {
		t.Fatalf("same inputs produced different sigs: %q vs %q", sig1, sig1Again)
	}
}

func TestSignIrisRequestEmptyBody(t *testing.T) {
	t.Parallel()

	sig := mustSignIrisRequest(t, "secret", "GET", "/config", "1711600000000", "nonce1", "")
	if sig == "" {
		t.Fatal("signature should not be empty for empty body")
	}

	if len(sig) != 64 {
		t.Fatalf("signature length = %d, want 64 hex chars", len(sig))
	}
}

func TestSignIrisRequestMethodCaseInsensitive(t *testing.T) {
	t.Parallel()

	sig1 := mustSignIrisRequest(t, "secret", "get", "/config", "123", "n", "")
	sig2 := mustSignIrisRequest(t, "secret", "GET", "/config", "123", "n", "")

	if sig1 != sig2 {
		t.Fatal("method should be case-insensitive (uppercased in canonical form)")
	}
}

func TestSignIrisRequestCanonicalizesEncodedQueryParams(t *testing.T) {
	t.Parallel()

	rawTarget := "/query?symbols=a%26b%3Dc%25&room%20name=%ED%95%9C%EA%B8%80%20%EC%B1%84%ED%8C%85"
	canonicalTarget := "/query?room%20name=%ED%95%9C%EA%B8%80%20%EC%B1%84%ED%8C%85&symbols=a%26b%3Dc%25"

	got := mustSignIrisRequest(t, "secret", "GET", rawTarget, "6000", "canon-n1", "")
	want := mustSignIrisRequest(t, "secret", "GET", canonicalTarget, "6000", "canon-n1", "")

	if got != want {
		t.Fatalf("signature mismatch for canonicalized query target:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	n1 := signing.GenerateNonce()
	n2 := signing.GenerateNonce()

	if n1 == n2 {
		t.Fatal("two consecutive nonces should differ")
	}

	if len(n1) != 32 {
		t.Fatalf("nonce length = %d, want 32 hex chars (16 bytes)", len(n1))
	}
}

func TestAPIClientHMACHeaders(t *testing.T) {
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

	c := NewAPIClient(server.URL, "my-token",
		WithTransport(transportHTTP1),
		WithHMACSecret("test-secret"),
	)

	if err := c.SendMessage(t.Context(), testRoom, "msg"); err != nil {
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

func TestAPIClientHMACHeadersOnGET(t *testing.T) {
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
			User:    ConfigState{BotName: testBotName},
			Applied: ConfigState{BotName: testBotName},
		}
		if err := jsonv2.MarshalWrite(w, resp); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	c := NewAPIClient(server.URL, "my-token",
		WithTransport(transportHTTP1),
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

func TestAPIClientNewRequestSignsWithBodyHash(t *testing.T) {
	t.Parallel()

	const hmacSecret = "new-request-secret"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "empty GET body",
			method: http.MethodGet,
			path:   PathReady,
		},
		{
			name:   "non-empty POST body",
			method: http.MethodPost,
			path:   PathDiagnosticsTextPing + "/123",
			body:   `{"message":"hello"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := NewAPIClient("http://localhost", "",
				WithTransport(transportHTTP1),
				WithHMACSecret(hmacSecret),
			)

			req, err := c.newSignedRequest(t.Context(), tt.method, tt.path, []byte(tt.body), SecretRoleBotControl)
			if err != nil {
				t.Fatalf("newSignedRequest() error = %v", err)
			}

			bodyHash := sha256.Sum256([]byte(tt.body))
			wantBodyHash := hex.EncodeToString(bodyHash[:])

			if got := req.Header.Get(HeaderIrisBodySHA256); got != wantBodyHash {
				t.Fatalf("X-Iris-Body-Sha256 = %q, want %q", got, wantBodyHash)
			}

			wantSignature := mustSignIrisRequestWithBodySHA256(t,
				hmacSecret,
				tt.method,
				tt.path,
				req.Header.Get(HeaderIrisTimestamp),
				req.Header.Get(HeaderIrisNonce),
				wantBodyHash,
			)
			if got := req.Header.Get(HeaderIrisSignature); got != wantSignature {
				t.Fatalf("X-Iris-Signature = %q, want %q", got, wantSignature)
			}

			if tt.body != "" {
				gotBody, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll(req.Body) error = %v", err)
				}

				if string(gotBody) != tt.body {
					t.Fatalf("request body = %q, want %q", string(gotBody), tt.body)
				}
			}
		})
	}
}

func TestAPIClientBotTokenSignsWhenNoHMAC(t *testing.T) {
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

	c := NewAPIClient(server.URL, "plain-token", WithTransport(transportHTTP1))

	if err := c.SendMessage(t.Context(), testRoom, "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotTimestamp == "" {
		t.Fatal("X-Iris-Timestamp header missing when signing with bot token")
	}

	if gotSignature == "" {
		t.Fatal("X-Iris-Signature header missing when signing with bot token")
	}
}

func TestAPIClientBotTokenSignsGETWhenNoHMAC(t *testing.T) {
	t.Parallel()

	var (
		gotTimestamp string
		gotSignature string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimestamp = r.Header.Get(HeaderIrisTimestamp)
		gotSignature = r.Header.Get(HeaderIrisSignature)

		if err := jsonv2.MarshalWrite(w, RoomListResponse{}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	c := NewAPIClient(server.URL, "my-token", WithTransport(transportHTTP1))

	if _, err := c.GetRooms(t.Context()); err != nil {
		t.Fatalf("GetRooms() error = %v", err)
	}

	if gotTimestamp == "" {
		t.Fatal("X-Iris-Timestamp header missing on GET")
	}

	if gotSignature == "" {
		t.Fatal("X-Iris-Signature header missing on GET")
	}
}

func TestAPIClientHMACSignatureVerifiable(t *testing.T) {
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

	c := NewAPIClient(server.URL, "token",
		WithTransport(transportHTTP1),
		WithHMACSecret(hmacSecret),
	)

	if err := c.SendMessage(t.Context(), testRoom, "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	// 캡처된 값으로 기대 서명을 재계산한다.
	expected := mustSignIrisRequest(t,
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

func TestAPIClientMultipartHMACSignsFullBody(t *testing.T) {
	t.Parallel()

	const hmacSecret = "verify-secret"

	var (
		capturedTimestamp string
		capturedNonce     string
		capturedSignature string
		capturedBodyHash  string
		capturedMethod    string
		capturedPath      string
		capturedMetadata  replyImageMetadata
		capturedBody      string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedTimestamp = r.Header.Get(HeaderIrisTimestamp)
		capturedNonce = r.Header.Get(HeaderIrisNonce)
		capturedSignature = r.Header.Get(HeaderIrisSignature)
		capturedBodyHash = r.Header.Get(HeaderIrisBodySHA256)
		capturedBody, capturedMetadata = readRawBodyAndMultipartReply(t, r)

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{Success: true, Delivery: testDeliveryAsync, RequestID: "req-hmac", Room: testRoom, Type: msgTypeImage}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	c := NewAPIClient(server.URL, "token",
		WithTransport(transportHTTP1),
		WithHMACSecret(hmacSecret),
	)

	if _, err := c.SendImage(t.Context(), testRoom, []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	expected := mustSignIrisRequest(t,
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

	bodyHash := sha256.Sum256([]byte(capturedBody))
	if capturedBodyHash != hex.EncodeToString(bodyHash[:]) {
		t.Fatalf("body hash = %q, want full multipart body hash", capturedBodyHash)
	}

	if capturedMetadata.Type != msgTypeImage || capturedMetadata.Room != testRoom {
		t.Fatalf("unexpected metadata: %+v", capturedMetadata)
	}
}

func readRawBodyAndMultipartReply(t *testing.T, r *http.Request) (string, replyImageMetadata) {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if err := r.Body.Close(); err != nil {
		t.Fatalf("body.Close() error = %v", err)
	}

	r.Body = io.NopCloser(strings.NewReader(string(body)))

	metadata, _ := readMultipartReplyRequest(t, r)

	return string(body), metadata
}

func TestAPIClientNoAuthHeadersWhenBothEmpty(t *testing.T) {
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

	c := NewAPIClient("http://localhost", "", WithRoundTripper(rt))

	if err := c.SendMessage(t.Context(), testRoom, "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}
