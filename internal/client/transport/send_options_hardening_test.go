package transport

import (
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

	if err := validateSendOptions(sendOptions{ThreadID: &threadID, ClientRequestID: &clientRequestID}); err != nil {
		t.Fatalf("validateSendOptions() error = %v, want nil", err)
	}
}

func TestValidateSendOptionsRejectsBlankThreadIDForScopedReply(t *testing.T) {
	t.Parallel()

	threadID := ""
	err := validateSendOptions(sendOptions{ThreadID: &threadID})

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
