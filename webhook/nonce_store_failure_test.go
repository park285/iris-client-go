package webhook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var errNonceStoreDown = errors.New("nonce store unavailable")

type failingNonceStore struct {
	mu    sync.Mutex
	calls int
}

func (s *failingNonceStore) IsDuplicate(_ context.Context, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++

	return false, errNonceStoreDown
}

func (*failingNonceStore) SetOnceNonce() {}

func (s *failingNonceStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls
}

type stallingNonceStore struct{}

func (stallingNonceStore) IsDuplicate(ctx context.Context, _ string, _ time.Duration) (bool, error) {
	<-ctx.Done()

	return false, ctx.Err()
}

func (stallingNonceStore) SetOnceNonce() {}

func nonceFailureRequest(t *testing.T, nonce string, timestamp time.Time) *http.Request {
	t.Helper()

	return signedWebhookRequest(t, "token", timestamp, nonce, testWebhookBody)
}

func TestWebhookHMACNonceStoreErrorReturnsServiceUnavailableWithoutAdmission(t *testing.T) {
	t.Parallel()

	var logs lockedBuffer
	store := &failingNonceStore{}
	admitter := &recordingAdmitter{}
	metrics := &mockMetrics{}
	handler := mustNewDurableHandler(t, admitter, slog.New(slog.NewJSONHandler(&logs, nil)),
		WithNonceStore(store),
		WithMetrics(metrics),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, nonceFailureRequest(t, "nonce-store-error", time.Now()))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if store.callCount() != 1 {
		t.Fatalf("nonce store calls = %d, want 1", store.callCount())
	}
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 because the nonce check failed before admission", admitter.calls)
	}
	if got := metrics.unauthorized.Load(); got != 0 {
		t.Fatalf("unauthorized metric = %d, want 0 for a store failure", got)
	}
	if got := handler.NonceStoreUnavailableCount(); got != 1 {
		t.Fatalf("NonceStoreUnavailableCount() = %d, want 1 for a store failure", got)
	}
	if !strings.Contains(logs.String(), "webhook hmac nonce check failed") {
		t.Fatalf("missing nonce failure warning, logs: %s", logs.String())
	}
}

func TestWebhookHMACNonceStoreTimeoutReturnsServiceUnavailableWithoutAdmission(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := mustNewDurableHandler(t, admitter, slog.New(slog.NewJSONHandler(&lockedBuffer{}, nil)),
		WithNonceStore(stallingNonceStore{}),
		WithDedupTimeout(20*time.Millisecond),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, nonceFailureRequest(t, "nonce-store-timeout", time.Now()))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 because the nonce check timed out before admission", admitter.calls)
	}
	if got := handler.NonceStoreUnavailableCount(); got != 1 {
		t.Fatalf("NonceStoreUnavailableCount() = %d, want 1 for a store timeout", got)
	}
}

func TestWebhookHMACNonceReplayStillReturnsUnauthorizedAfterAdmission(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	metrics := &mockMetrics{}
	handler := mustNewDurableHandler(t, admitter, slog.New(slog.NewJSONHandler(&lockedBuffer{}, nil)),
		WithNonceStore(newMemoryNonceCache()),
		WithMetrics(metrics),
	)
	defer closeHandler(t, handler)

	now := time.Now()
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, nonceFailureRequest(t, "nonce-real-replay", now))
	assertResponseCode(t, firstRecorder.Code, http.StatusOK)
	if admitter.calls != 1 {
		t.Fatalf("first admission calls = %d, want 1", admitter.calls)
	}

	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, nonceFailureRequest(t, "nonce-real-replay", now))

	assertResponseCode(t, secondRecorder.Code, http.StatusUnauthorized)
	if admitter.calls != 1 {
		t.Fatalf("admission calls after replay = %d, want 1", admitter.calls)
	}
	if got := metrics.unauthorized.Load(); got != 1 {
		t.Fatalf("unauthorized metric = %d, want 1 for a real replay", got)
	}
	if got := handler.NonceStoreUnavailableCount(); got != 0 {
		t.Fatalf("NonceStoreUnavailableCount() = %d, want 0 for a real replay", got)
	}
}

func TestWebhookHMACNilNonceCacheStaysFailClosed(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	metrics := &mockMetrics{}
	handler := mustNewDurableHandler(t, admitter, slog.New(slog.NewJSONHandler(&lockedBuffer{}, nil)),
		WithNonceStore(newMemoryNonceCache()),
		WithMetrics(metrics),
	)
	defer closeHandler(t, handler)
	handler.nonceStore = nil

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, nonceFailureRequest(t, "nonce-nil-cache", time.Now()))

	assertResponseCode(t, recorder.Code, http.StatusUnauthorized)
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 with a nil nonce cache", admitter.calls)
	}
	if got := metrics.unauthorized.Load(); got != 1 {
		t.Fatalf("unauthorized metric = %d, want 1 with a nil nonce cache", got)
	}
	if got := handler.NonceStoreUnavailableCount(); got != 0 {
		t.Fatalf("NonceStoreUnavailableCount() = %d, want 0 with a nil nonce cache", got)
	}
}
