package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 인증은 통과했지만 본문이 깨진 요청이 header의 message ID를 admission에 남기면, 같은 ID의
// 정상 전송이 durable unique key 충돌로 duplicate 처리되어 유실된다.
func TestMalformedV3RequestCannotPoisonMessageIDAdmission(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	const messageID = "mid-poison-regression"

	malformed := newV3MessageIDSecurityRequest(t, "{invalid-json", messageID, "poison-attempt")
	malformedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(malformedRecorder, malformed)

	if malformedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want %d", malformedRecorder.Code, http.StatusBadRequest)
	}

	if admitter.calls != 0 {
		t.Fatalf("malformed request reached admission %d times", admitter.calls)
	}

	validBody := validJSONBodyWithMessageID(messageID)
	valid := newV3MessageIDSecurityRequest(t, validBody, messageID, "valid-delivery")
	validRecorder := httptest.NewRecorder()
	handler.ServeHTTP(validRecorder, valid)

	if validRecorder.Code != http.StatusOK {
		t.Fatalf("valid status = %d, want %d", validRecorder.Code, http.StatusOK)
	}

	if admitter.calls != 1 || admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != messageID {
		t.Fatalf("admission = calls:%d msg:%#v, want one message with ID %q", admitter.calls, admitter.msg, messageID)
	}
}

func newV3MessageIDSecurityRequest(t *testing.T, body, messageID, nonce string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathWebhook, strings.NewReader(body))
	request.Header.Set("Content-Type", contentTypeJSON)
	request.Header.Set(HeaderIrisMessageID, messageID)
	signWebhookTestRequest(t, request, testToken, time.Now(), nonce, []byte(body))

	return request
}
