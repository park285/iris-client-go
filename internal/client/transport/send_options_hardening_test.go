package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplySendOptionsIgnoresNilOption(t *testing.T) {
	t.Parallel()

	got := applySendOptions([]SendOption{nil, WithThreadID("123")})
	if got.ThreadID == nil || *got.ThreadID != "123" {
		t.Fatalf("ThreadID = %v, want 123", got.ThreadID)
	}
}

func TestValidateSendOptionsNormalizesThreadAndClientRequestID(t *testing.T) {
	t.Parallel()

	threadID := " 12345 "
	clientRequestID := " chatbotgo:log-42:reply-v1 "
	threadScope := 2
	if err := validateSendOptions(sendOptions{ThreadID: &threadID, ThreadScope: &threadScope, ClientRequestID: &clientRequestID}); err != nil {
		t.Fatalf("validateSendOptions() error = %v, want nil", err)
	}
}

func TestValidateSendOptionsRejectsBlankThreadIDForScopedReply(t *testing.T) {
	t.Parallel()

	threadID := ""
	threadScope := 2
	err := validateSendOptions(sendOptions{ThreadID: &threadID, ThreadScope: &threadScope})
	if err == nil || err.Error() != "iris: threadId must not be blank" {
		t.Fatalf("validateSendOptions() error = %v, want blank threadId error", err)
	}
}

func TestValidateSendOptionsRejectsNonASCIIThreadID(t *testing.T) {
	t.Parallel()

	threadID := "１２３"
	err := validateSendOptions(sendOptions{ThreadID: &threadID})
	if err == nil || !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("validateSendOptions() error = %v, want numeric threadId error", err)
	}
}

func TestValidateSendOptionsRejectsInvalidAttachmentJSON(t *testing.T) {
	t.Parallel()

	for _, raw := range []json.RawMessage{
		json.RawMessage(` `),
		json.RawMessage(`{"broken"`),
		json.RawMessage(`[1,2,3]`),
		json.RawMessage(`null`),
	} {
		if err := validateAttachmentJSON(raw, false); err == nil {
			t.Fatalf("validateAttachmentJSON(%q) error = nil, want error", string(raw))
		}
	}
}

func TestWithAttachmentJSONClonesInput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"a":1}`)
	opt := WithAttachmentJSON(raw)
	raw[2] = 'x'

	got := applySendOptions([]SendOption{opt})
	if string(got.AttachmentJSON) != `{"a":1}` {
		t.Fatalf("AttachmentJSON = %s, want cloned original", got.AttachmentJSON)
	}
}

func TestNonTextRepliesRejectAttachmentJSON(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://example.com", "", WithTransport("http1"))
	if _, err := client.SendMarkdown(t.Context(), "room", "**hello**", WithAttachmentJSON(json.RawMessage(`{"a":1}`))); err == nil || !strings.Contains(err.Error(), "attachmentJson requires text reply type") {
		t.Fatalf("SendMarkdown() error = %v, want attachment/text validation error", err)
	}
	if _, err := client.SendImage(t.Context(), "room", []byte("image"), WithAttachmentJSON(json.RawMessage(`{"a":1}`))); err == nil || !strings.Contains(err.Error(), "attachmentJson requires text reply type") {
		t.Fatalf("SendImage() error = %v, want attachment/text validation error", err)
	}
}
