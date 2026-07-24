package webhook

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIC04WebhookDedupAfterDecodeCannotPoisonMessageID_9a32d3ef(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := "{invalid-json"
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "victim-message-id")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (malformed body must be rejected before dedup)", recorder.Code, http.StatusBadRequest)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup was called %d times for a malformed body; header-only poisoning is possible", len(calls))
	}
}

func TestIC04WebhookRejectsMismatchedBodyAndHeaderMessageID_9a32d3ef(t *testing.T) {
	t.Parallel()

	dedup := &mockDeduplicator{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	body := `{"text":"hi","room":"room-1","userId":"user-1","messageId":"canonical-from-body"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "spoofed-header-id")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup call count = %d, want 0 for mismatched identities", len(calls))
	}
}

func TestIC04WebhookRejectsOversizeEventPayloadEvenWithinBodyLimit_3e9c9876(t *testing.T) {
	t.Parallel()

	oversize := strings.Repeat("a", maxEventPayloadBytes+1)
	body := fmt.Sprintf(`{"room":"room-1","userId":"user-1","type":"event","eventPayload":{"blob":%q}}`, oversize)
	if int64(len(body)) >= defaultMaxBodyBytes {
		t.Fatalf("test body %d bytes must stay under MaxBodyBytes %d to isolate the EventPayload cap", len(body), defaultMaxBodyBytes)
	}

	metrics := &mockMetrics{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithMetrics(metrics),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "evt-1")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (oversize EventPayload within body limit must be rejected)", recorder.Code, http.StatusBadRequest)
	}
}

func TestIC04WebhookAcceptsEventPayloadWithinCap_3e9c9876(t *testing.T) {
	t.Parallel()

	body := `{"room":"room-1","userId":"user-1","type":"event","eventPayload":{"k":"v"}}`
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "evt-ok")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (small EventPayload must be accepted)", recorder.Code, http.StatusOK)
	}
}
