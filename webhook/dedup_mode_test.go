package webhook

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeHTTPDedupAfterDecodeRejectsMalformedWithoutDedupCall(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeAfterDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader("{invalid-json"))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-malformed")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", "{invalid-json")

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusBadRequest)
	assertMetricCounts(t, metrics, metricCounts{requests: 1, badRequest: 1})
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup calls = %d, want 0", len(calls))
	}
}

func TestServeHTTPDedupAfterDecodeStillDropsValidDuplicate(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeAfterDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-dup-after-decode"))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-dup-after-decode")

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)
	assertMetricCounts(t, metrics, metricCounts{requests: 1, duplicate: 1})
	if calls := dedup.snapshot(); len(calls) != 1 {
		t.Fatalf("dedup calls = %d, want 1", len(calls))
	}

	select {
	case msg := <-capture.msgCh:
		t.Fatalf("unexpected enqueue for duplicate: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}
}
