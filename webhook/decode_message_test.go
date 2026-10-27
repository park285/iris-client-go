package webhook

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

// inbox는 handler가 admitter에 넘긴 Iris 원본 본문을 저장하고 DecodeMessage로 복원한다. Iris 계약
// vector마다 복원 결과가 admission 때 받은 Message와 같아야 저장 행이 SDK 구조체 변경과 무관해진다.
func TestDecodeMessageRestoresAdmittedMessageFromIrisVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/webhook_payload_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}

	var vectors []struct {
		Name        string `json:"name"`
		PayloadJSON string `json:"payloadJson"`
	}

	if err := jsonv2.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}

	if len(vectors) == 0 {
		t.Fatal("no webhook payload vectors")
	}

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()

			var envelope struct {
				MessageID string `json:"messageId"`
			}

			if err := jsonv2.Unmarshal([]byte(vector.PayloadJSON), &envelope); err != nil {
				t.Fatalf("decode messageId: %v", err)
			}

			admitter := &recordingAdmitter{}
			handler := newTestHandler(t, admitter)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, newSignedIdentityRequest(t, vector.PayloadJSON, envelope.MessageID))

			if recorder.Code != http.StatusOK || admitter.calls != 1 {
				t.Fatalf("status = %d, admits = %d, want 200 and one admission", recorder.Code, admitter.calls)
			}

			if !bytes.Equal(admitter.body, []byte(vector.PayloadJSON)) {
				t.Fatalf("admitted body = %s, want the original Iris body", admitter.body)
			}

			restored, err := DecodeMessage(admitter.body)
			if err != nil {
				t.Fatalf("DecodeMessage() error = %v", err)
			}

			if !reflect.DeepEqual(restored, admitter.msg) {
				t.Fatalf("DecodeMessage() = %#v / %#v, want admitted %#v / %#v", restored, restored.JSON, admitter.msg, admitter.msg.JSON)
			}
		})
	}
}

// DecodeMessage는 handler와 같은 계약 검증을 적용한다. 퇴역한 mention user_id와 숫자 userId, 필수 field
// 누락은 복원하지 않는다.
func TestDecodeMessageRejectsBodiesOutsideIrisContract(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"retired mention user_id": `{"messageId":"m-1","text":"hi","room":"r","userId":"u","mentions":[{"user_id":"1"}]}`,
		"numeric mention userId":  `{"messageId":"m-1","text":"hi","room":"r","userId":"u","mentions":[{"userId":1}]}`,
		"missing room":            `{"messageId":"m-1","text":"hi","userId":"u"}`,
		"missing messageId":       `{"text":"hi","room":"r","userId":"u"}`,
		"not JSON":                `{`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if msg, err := DecodeMessage([]byte(body)); err == nil {
				t.Fatalf("DecodeMessage(%s) = %#v, want error", body, msg)
			}
		})
	}

	if _, err := DecodeMessage([]byte(`{"messageId":"m-1","text":"hi","userId":"u"}`)); !errors.Is(err, errInvalidWebhookRequest) {
		t.Fatalf("DecodeMessage(missing room) error = %v, want errInvalidWebhookRequest", err)
	}
}
