package webhook

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

func TestWebhookRejectsInvalidMessageIDHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		messageID string
	}{
		{name: "over 256 bytes", messageID: strings.Repeat("a", 257)},
		{name: "outside identifier grammar", messageID: "message id@invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			admitter := &recordingAdmitter{}
			handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
			defer closeHandler(t, handler)

			request := newV2IdentityRequest(t, validJSONBody(), tt.messageID)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
			if admitter.calls != 0 {
				t.Fatalf("admission calls = %d, want 0", admitter.calls)
			}
		})
	}
}

func TestWebhookTrimsMatchingBodyAndHeaderMessageID(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := `{"messageId":" message-1 ","text":"hello","room":"room-1","userId":"user-1"}`
	request := newV2IdentityRequest(t, body, " message-1 ")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != "message-1" {
		t.Fatalf("admitted message = %#v, want normalized body identity", admitter.msg)
	}
}

func TestWebhookAcceptsCanonicalMessageIDBoundaries(t *testing.T) {
	t.Parallel()

	for _, messageID := range []string{
		strings.Repeat("a", maxMessageIDBytes),
		"kakao-log-g1-1000000000001-alerts/main",
	} {
		messageID := messageID
		t.Run(messageID[:min(len(messageID), 32)], func(t *testing.T) {
			t.Parallel()

			admitter := &recordingAdmitter{}
			handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
			defer closeHandler(t, handler)

			body := fmt.Sprintf(`{"messageId":%q,"text":"hello","room":"room-1","userId":"user-1"}`, messageID)
			request := newV2IdentityRequest(t, body, messageID)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != messageID {
				t.Fatalf("admitted message = %#v, want message ID %q", admitter.msg, messageID)
			}
		})
	}
}

func TestWebhookRejectsBodyHeaderMessageIDMismatch(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := `{"messageId":"body-message-id","text":"hello","room":"room-1","userId":"user-1"}`
	request := newV2IdentityRequest(t, body, "header-message-id")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0", admitter.calls)
	}
}

func TestWebhookV2AcceptsAuthenticatedHeaderMessageID(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := validJSONBody()
	request := newV2IdentityRequest(t, body, " message-v2 ")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != "message-v2" {
		t.Fatalf("admitted message = %#v, want authenticated v2 header identity", admitter.msg)
	}
}

func TestWebhookV2AuthenticatedIdentityConsumesNonce(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := validJSONBody()
	first := newV2IdentityRequest(t, body, "message-v2-replay")
	second := httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathWebhook, strings.NewReader(body))
	second.Header = first.Header.Clone()

	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRecorder.Code, http.StatusOK)
	}

	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("second status = %d, want %d", secondRecorder.Code, http.StatusUnauthorized)
	}
	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}
}

func TestWebhookV2MessageIDMutationInvalidatesSignature(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := validJSONBody()
	request := newV2IdentityRequest(t, body, "message-v2")
	request.Header.Set(HeaderIrisMessageID, "message-v2-mutated")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0", admitter.calls)
	}
}

func TestWebhookV2RejectsBodyHeaderMessageIDMismatch(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := `{"messageId":"body-message-id","text":"hello","room":"room-1","userId":"user-1"}`
	request := newV2IdentityRequest(t, body, "header-message-id")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0", admitter.calls)
	}
}

func TestWebhookRejectsUnknownSignatureVersion(t *testing.T) {
	t.Parallel()

	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	body := validJSONBody()
	request := newValidRequest(t, t.Context(), body)
	request.Header.Set(HeaderIrisSignatureVersion, "v99")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookRequiresSignatureVersion(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	request := newV2IdentityRequest(t, validJSONBody(), "message-v2")
	request.Header.Del(HeaderIrisSignatureVersion)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0", admitter.calls)
	}
}

func newV2IdentityRequest(t *testing.T, body, messageID string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathWebhook, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(HeaderIrisMessageID, messageID)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := "message-identity-v2-test"
	bodySHA256 := irishmac.SHA256HexBytes([]byte(body))
	target, err := irishmac.CanonicalTarget(request.URL.RequestURI())
	if err != nil {
		t.Fatalf("CanonicalTarget() error = %v", err)
	}
	canonical := strings.Join([]string{
		SignatureVersionV2,
		strings.ToUpper(request.Method),
		target,
		timestamp,
		nonce,
		strings.TrimSpace(messageID),
		bodySHA256,
	}, "\n")

	request.Header.Set(HeaderIrisSignatureVersion, SignatureVersionV2)
	request.Header.Set(HeaderIrisTimestamp, timestamp)
	request.Header.Set(HeaderIrisNonce, nonce)
	request.Header.Set(HeaderIrisBodySHA256, bodySHA256)
	request.Header.Set(HeaderIrisSignature, irishmac.NewSigner("token").Sign(canonical))

	return request
}
