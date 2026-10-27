package webhook

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIC04WebhookRejectsMismatchedBodyAndHeaderMessageID_9a32d3ef(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	body := `{"text":"hi","room":"room-1","userId":"user-1","messageId":"canonical-from-body"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "spoofed-header-id")
	request.Header.Set("Content-Type", contentTypeJSON)
	signHandlerTestRequest(t, request, testToken, body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 for mismatched identities", admitter.calls)
	}
}

func TestIC04WebhookRejectsOversizeEventPayloadEvenWithinBodyLimit_3e9c9876(t *testing.T) {
	t.Parallel()

	oversize := strings.Repeat("a", maxEventPayloadBytes+1)
	body := fmt.Sprintf(`{"room":"room-1","userId":"user-1","type":"event","eventPayload":{"blob":%q}}`, oversize)

	if int64(len(body)) >= defaultMaxBodyBytes {
		t.Fatalf("test body %d bytes must stay under MaxBodyBytes %d to isolate the EventPayload cap", len(body), defaultMaxBodyBytes)
	}

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter, WithMetrics(&mockMetrics{}))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "evt-1")
	request.Header.Set("Content-Type", contentTypeJSON)
	signHandlerTestRequest(t, request, testToken, body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (oversize EventPayload within body limit must be rejected)", recorder.Code, http.StatusBadRequest)
	}

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 for an oversize EventPayload", admitter.calls)
	}
}

func TestIC04WebhookAcceptsEventPayloadWithinCap_3e9c9876(t *testing.T) {
	t.Parallel()

	body := `{"messageId":"evt-ok","room":"room-1","userId":"user-1","type":"event","eventPayload":{"k":"v"}}`
	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, "evt-ok")
	request.Header.Set("Content-Type", contentTypeJSON)
	signHandlerTestRequest(t, request, testToken, body)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (small EventPayload must be accepted)", recorder.Code, http.StatusOK)
	}

	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}
}
