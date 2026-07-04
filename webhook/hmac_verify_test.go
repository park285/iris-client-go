package webhook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const (
	testWebhookToken  = "legacy-webhook-token"
	testWebhookSecret = "signed-webhook-secret"
)

var testWebhookBody = []byte(`{"room":"room","sender":"sender","userId":"user","text":"hello"}`)

type hmacVerifyHandler struct{}

func (hmacVerifyHandler) HandleMessage(context.Context, *Message) {}

func TestWebhookHMACVerifyValidSignature(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	req := signedWebhookRequest(t, testWebhookSecret, time.Now(), "nonce-valid", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestWebhookHMACVerifyAbsentSignatureHeadersRequireOffFallsBackToToken(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t)
	req := unsignedWebhookRequest(testWebhookBody)
	req.Header.Set(HeaderIrisToken, testWebhookToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestWebhookHMACVerifyAbsentSignatureHeadersRequireOnRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithRequireHMAC(true))
	req := unsignedWebhookRequest(testWebhookBody)
	req.Header.Set(HeaderIrisToken, testWebhookToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyExpiredTimestampRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t,
		WithWebhookSecret(testWebhookSecret),
		WithRequireHMAC(true),
		WithReplayWindow(time.Minute),
	)
	req := signedWebhookRequest(t, testWebhookSecret, time.Now().Add(-10*time.Minute), "nonce-expired", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyNonceReuseRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	now := time.Now()
	first := signedWebhookRequest(t, testWebhookSecret, now, "nonce-reuse", testWebhookBody)
	second := signedWebhookRequest(t, testWebhookSecret, now, "nonce-reuse", testWebhookBody)
	firstRecorder := httptest.NewRecorder()
	secondRecorder := httptest.NewRecorder()

	handler.ServeHTTP(firstRecorder, first)
	handler.ServeHTTP(secondRecorder, second)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRecorder.Code, http.StatusOK)
	}
	if secondRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("second status = %d, want %d", secondRecorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyBodySHA256MismatchRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	req := signedWebhookRequestWithBodyHash(t, testWebhookSecret, time.Now(), "nonce-body-mismatch", testWebhookBody, irishmac.EmptyBodySHA256Hex)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyBadSignatureRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	req := signedWebhookRequest(t, testWebhookSecret, time.Now(), "nonce-bad-signature", testWebhookBody)
	req.Header.Set(HeaderIrisSignature, strings.Repeat("0", 64))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func newHMACVerifyTestHandler(t *testing.T, opts ...HandlerOption) *Handler {
	t.Helper()

	handler := NewHandler(t.Context(), testWebhookToken, hmacVerifyHandler{}, nil, opts...)
	t.Cleanup(func() {
		if err := handler.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return handler
}

func unsignedWebhookRequest(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, PathWebhook, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func signedWebhookRequest(t *testing.T, secret string, timestamp time.Time, nonce string, body []byte) *http.Request {
	t.Helper()

	return signedWebhookRequestWithBodyHash(t, secret, timestamp, nonce, body, irishmac.SHA256HexBytes(body))
}

func signedWebhookRequestWithBodyHash(t *testing.T, secret string, timestamp time.Time, nonce string, body []byte, bodySHA256 string) *http.Request {
	t.Helper()

	req := unsignedWebhookRequest(body)
	timestampMs := strconv.FormatInt(timestamp.UnixMilli(), 10)
	signature, err := irishmac.SignCanonical(
		irishmac.NewSigner(secret),
		req.Method,
		req.URL.RequestURI(),
		timestampMs,
		nonce,
		bodySHA256,
	)
	if err != nil {
		t.Fatalf("SignCanonical() error = %v", err)
	}

	req.Header.Set(HeaderIrisTimestamp, timestampMs)
	req.Header.Set(HeaderIrisNonce, nonce)
	req.Header.Set(HeaderIrisBodySHA256, bodySHA256)
	req.Header.Set(HeaderIrisSignature, signature)
	return req
}
