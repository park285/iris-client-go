package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

func TestWebhookHMACVerifyAbsentSignatureHeadersRejectsTokenOnly(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t)
	req := unsignedWebhookRequest(testWebhookBody)
	req.Header.Set(HeaderIrisToken, testWebhookToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyAbsentSignatureHeadersRejectsWhenOptionExplicitlyFalse(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithRequireHMAC(false))
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

func TestWebhookRejectedBodyReservesNonce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed JSON", body: []byte(`{"room":`)},
		{name: "invalid schema", body: []byte(`{"text":"hello","userId":"user"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
			now := time.Now()
			nonce := "nonce-rejected-body-" + strings.ReplaceAll(tt.name, " ", "-")
			first := signedWebhookRequest(t, testWebhookSecret, now, nonce, tt.body)
			second := signedWebhookRequest(t, testWebhookSecret, now, nonce, tt.body)

			firstRecorder := httptest.NewRecorder()
			handler.ServeHTTP(firstRecorder, first)
			if firstRecorder.Code != http.StatusBadRequest {
				t.Fatalf("first status = %d, want %d", firstRecorder.Code, http.StatusBadRequest)
			}

			secondRecorder := httptest.NewRecorder()
			handler.ServeHTTP(secondRecorder, second)
			if secondRecorder.Code != http.StatusUnauthorized {
				t.Fatalf("second status = %d, want %d", secondRecorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestWebhookConcurrentEnvelopeAllowsOneRequest(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	now := time.Now()
	const (
		attempts = 16
		nonce    = "nonce-concurrent-envelope"
	)

	requests := make([]*http.Request, attempts)
	for i := range requests {
		requests[i] = signedWebhookRequest(t, testWebhookSecret, now, nonce, testWebhookBody)
	}

	start := make(chan struct{})
	results := make(chan int, attempts)
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(request *http.Request) {
			defer wg.Done()
			<-start
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			results <- recorder.Code
		}(request)
	}
	close(start)
	wg.Wait()
	close(results)

	statusCounts := make(map[int]int)
	for status := range results {
		statusCounts[status]++
	}
	if statusCounts[http.StatusOK] != 1 || statusCounts[http.StatusUnauthorized] != attempts-1 {
		t.Fatalf("status counts = %v, want one %d and %d %d responses", statusCounts, http.StatusOK, attempts-1, http.StatusUnauthorized)
	}
}

func TestWebhookRejectedIdentityDoesNotReserveNonce(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret), WithRequireHMAC(true))
	body := []byte(`{"messageId":"body-message-id","room":"room","sender":"sender","userId":"user","text":"hello"}`)
	now := time.Now()
	const nonce = "nonce-rejected-identity"

	mutated := signedWebhookRequest(t, testWebhookSecret, now, nonce, body)
	mutated.Header.Set(HeaderIrisMessageID, "mutated-message-id")
	mutatedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(mutatedRecorder, mutated)
	if mutatedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("mutated status = %d, want %d", mutatedRecorder.Code, http.StatusUnauthorized)
	}

	original := signedWebhookRequest(t, testWebhookSecret, now, nonce, body)
	originalRecorder := httptest.NewRecorder()
	handler.ServeHTTP(originalRecorder, original)
	if originalRecorder.Code != http.StatusOK {
		t.Fatalf("original status = %d, want %d", originalRecorder.Code, http.StatusOK)
	}

	replay := signedWebhookRequest(t, testWebhookSecret, now, nonce, body)
	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want %d", replayRecorder.Code, http.StatusUnauthorized)
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

func TestWebhookHMACVerifyPartialSignatureHeadersRejectsDespiteValidToken(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret))
	req := unsignedWebhookRequest(testWebhookBody)
	req.Header.Set(HeaderIrisToken, testWebhookToken)
	req.Header.Set(HeaderIrisTimestamp, strconv.FormatInt(time.Now().UnixMilli(), 10))
	req.Header.Set(HeaderIrisNonce, "nonce-partial")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyPresentButInvalidSignatureNotDowngradedToToken(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithWebhookSecret(testWebhookSecret))
	req := signedWebhookRequest(t, testWebhookSecret, time.Now(), "nonce-nodowngrade", testWebhookBody)
	req.Header.Set(HeaderIrisSignature, strings.Repeat("0", 64))
	req.Header.Set(HeaderIrisToken, testWebhookToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyFutureTimestampWithinWindowAccepts(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t,
		WithWebhookSecret(testWebhookSecret),
		WithRequireHMAC(true),
		WithReplayWindow(time.Minute),
	)
	req := signedWebhookRequest(t, testWebhookSecret, time.Now().Add(30*time.Second), "nonce-future-ok", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestWebhookHMACVerifyFutureTimestampOutsideWindowRejects(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t,
		WithWebhookSecret(testWebhookSecret),
		WithRequireHMAC(true),
		WithReplayWindow(time.Minute),
	)
	req := signedWebhookRequest(t, testWebhookSecret, time.Now().Add(10*time.Minute), "nonce-future-bad", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHMACVerifyNonceTTLIsDoubleReplayWindow(t *testing.T) {
	t.Parallel()

	cache := &recordingNonceCache{}
	handler := newHMACVerifyTestHandler(t,
		WithWebhookSecret(testWebhookSecret),
		WithRequireHMAC(true),
		WithReplayWindow(time.Minute),
		WithNonceCache(cache),
	)
	req := signedWebhookRequest(t, testWebhookSecret, time.Now(), "nonce-ttl", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	_, ttls := cache.snapshot()
	if len(ttls) != 1 {
		t.Fatalf("nonce cache calls = %d, want 1", len(ttls))
	}
	if ttls[0] != 2*time.Minute {
		t.Fatalf("nonce ttl = %v, want %v", ttls[0], 2*time.Minute)
	}
}

func TestWebhookHMACVerifyNonceCacheSharesDeduplicatorBackend(t *testing.T) {
	t.Parallel()

	shared := &recordingNonceCache{}
	handler := newHMACVerifyTestHandler(t,
		WithWebhookSecret(testWebhookSecret),
		WithRequireHMAC(true),
		WithDeduplicator(shared),
	)
	req := signedWebhookRequest(t, testWebhookSecret, time.Now(), "nonce-shared", testWebhookBody)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	keys, _ := shared.snapshot()
	found := false
	for _, key := range keys {
		if strings.HasPrefix(key, http.MethodPost+"\n") {
			found = true
		}
	}
	if !found {
		t.Fatalf("shared deduplicator did not receive nonce key; got keys = %v", keys)
	}
}

func TestWebhookNonceCacheDefaultsToMemory(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t)
	assertMemoryNonceCache(t, handler)
}

func TestWebhookNonceCacheKeepsMemoryForNoopDeduplicatorValue(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithDeduplicator(NoopDeduplicator{}))
	assertMemoryNonceCache(t, handler)
}

func TestWebhookNonceCacheKeepsMemoryForNoopDeduplicatorPointer(t *testing.T) {
	t.Parallel()

	handler := newHMACVerifyTestHandler(t, WithDeduplicator(&NoopDeduplicator{}))
	assertMemoryNonceCache(t, handler)
}

func assertMemoryNonceCache(t *testing.T, handler *Handler) {
	t.Helper()

	if _, ok := handler.nonceCache.(*memoryNonceCache); !ok {
		t.Fatalf("nonceCache = %T, want *memoryNonceCache", handler.nonceCache)
	}
}

type recordingNonceCache struct {
	mu   sync.Mutex
	keys []string
	ttls []time.Duration
}

func (c *recordingNonceCache) IsDuplicate(_ context.Context, key string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	c.ttls = append(c.ttls, ttl)
	return false, nil
}

func (c *recordingNonceCache) snapshot() ([]string, []time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.keys...), append([]time.Duration(nil), c.ttls...)
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
	messageID := ensureWebhookTestMessageID(req, body)
	signWebhookTestRequestWithBodyHash(t, req, secret, timestamp, nonce, messageID, bodySHA256)
	return req
}

func signWebhookTestRequest(t *testing.T, req *http.Request, secret string, timestamp time.Time, nonce string, body []byte) {
	t.Helper()

	messageID := ensureWebhookTestMessageID(req, body)
	signWebhookTestRequestWithBodyHash(t, req, secret, timestamp, nonce, messageID, irishmac.SHA256HexBytes(body))
}

func signWebhookTestRequestWithBodyHash(t *testing.T, req *http.Request, secret string, timestamp time.Time, nonce, messageID, bodySHA256 string) {
	t.Helper()

	timestampMs := strconv.FormatInt(timestamp.UnixMilli(), 10)
	target, err := irishmac.CanonicalTarget(req.URL.RequestURI())
	if err != nil {
		t.Fatalf("CanonicalTarget() error = %v", err)
	}
	canonical := canonicalWebhookRequestV2(req.Method, target, timestampMs, nonce, messageID, bodySHA256)
	signature := irishmac.NewSigner(secret).Sign(canonical)

	req.Header.Set(HeaderIrisSignatureVersion, SignatureVersionV2)
	req.Header.Set(HeaderIrisTimestamp, timestampMs)
	req.Header.Set(HeaderIrisNonce, nonce)
	req.Header.Set(HeaderIrisBodySHA256, bodySHA256)
	req.Header.Set(HeaderIrisSignature, signature)
}

func ensureWebhookTestMessageID(req *http.Request, body []byte) string {
	messageID := strings.TrimSpace(req.Header.Get(HeaderIrisMessageID))
	if messageID == "" {
		var payload struct {
			MessageID string `json:"messageId"`
		}
		if json.Unmarshal(body, &payload) == nil {
			messageID = strings.TrimSpace(payload.MessageID)
		}
	}
	if messageID == "" {
		messageID = "webhook-test-message-id"
	}
	req.Header.Set(HeaderIrisMessageID, messageID)

	return messageID
}
