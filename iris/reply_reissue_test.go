package iris

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func conflictErr(code string) error {
	return fmt.Errorf("send: %w", &HTTPError{StatusCode: 409, Body: fmt.Sprintf(`{"code":%q}`, code)})
}

func TestReplyReissueSuffix(t *testing.T) {
	if got := ReplyReissueSuffix(0); got != "" {
		t.Fatalf("ReplyReissueSuffix(0) = %q, want empty", got)
	}
	if got := ReplyReissueSuffix(-1); got != "" {
		t.Fatalf("ReplyReissueSuffix(-1) = %q, want empty", got)
	}
	if got := ReplyReissueSuffix(1); got != ":r1" {
		t.Fatalf("ReplyReissueSuffix(1) = %q, want :r1", got)
	}
	if got := ReplyReissueSuffix(2); got != ":r2" {
		t.Fatalf("ReplyReissueSuffix(2) = %q, want :r2", got)
	}
}

func TestReissuedClientRequestID(t *testing.T) {
	got, err := ReissuedClientRequestID("chatbotgo:abc123", 1)
	if err != nil {
		t.Fatalf("ReissuedClientRequestID() error = %v", err)
	}
	if got != "chatbotgo:abc123:r1" {
		t.Fatalf("ReissuedClientRequestID() = %q, want chatbotgo:abc123:r1", got)
	}

	maxed, err := ReissuedClientRequestID("chatbotgo:abc123", ReplyReissueMaxGenerations)
	if err != nil {
		t.Fatalf("generation max error = %v, want nil", err)
	}
	if maxed != "chatbotgo:abc123:r2" {
		t.Fatalf("generation max = %q, want chatbotgo:abc123:r2", maxed)
	}

	if _, err := ReissuedClientRequestID("chatbotgo:abc123", 0); !errors.Is(err, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation 0 error = %v, want ErrReplyReissueGenerationOutOfRange", err)
	}
	if _, err := ReissuedClientRequestID("chatbotgo:abc123", -1); !errors.Is(err, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation -1 error = %v, want ErrReplyReissueGenerationOutOfRange", err)
	}
	if _, err := ReissuedClientRequestID("chatbotgo:abc123", ReplyReissueMaxGenerations+1); !errors.Is(err, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation over max error = %v, want ErrReplyReissueGenerationOutOfRange", err)
	}
	if _, err := ReissuedClientRequestID("", 1); err == nil {
		t.Fatal("empty base error = nil, want error")
	}
	if _, err := ReissuedClientRequestID(strings.Repeat("a", 159), 1); err == nil {
		t.Fatal("over-length candidate error = nil, want validation error")
	}

	atLimit, err := ReissuedClientRequestID(strings.Repeat("a", 157), 1)
	if err != nil {
		t.Fatalf("157-char base error = %v, want nil (candidate len 160)", err)
	}
	if len(atLimit) != 160 {
		t.Fatalf("candidate len = %d, want 160", len(atLimit))
	}
	if _, err := ReissuedClientRequestID(strings.Repeat("a", 158), 1); err == nil {
		t.Fatal("158-char base error = nil, want validation error (candidate len 161)")
	}

	if _, err := ReissuedClientRequestID("chatbotgo:abc 123", 1); err == nil {
		t.Fatal("invalid-charset base error = nil, want validation error")
	}

	if _, err := ReissuedClientRequestID("chatbotgo:abc123:r1", 2); !errors.Is(err, ErrReplyReissueBaseAlreadyReissued) {
		t.Fatalf("already-reissued base error = %v, want ErrReplyReissueBaseAlreadyReissued", err)
	}
}

func TestClientRequestIDConflictPredicates(t *testing.T) {
	preHandoff := conflictErr(HTTPErrorCodeClientRequestIDFailed)
	if !IsPreHandoffClientRequestIDConflict(preHandoff) {
		t.Fatal("CLIENT_REQUEST_ID_FAILED must be pre-handoff conflict")
	}
	if IsTerminalClientRequestIDConflict(preHandoff) {
		t.Fatal("CLIENT_REQUEST_ID_FAILED must not be terminal")
	}
	if !IsUnrecoverableClientRequestIDConflict(preHandoff) {
		t.Fatal("exhausted pre-handoff conflict must be unrecoverable")
	}

	for _, code := range []string{
		HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDAlreadyExists,
	} {
		err := conflictErr(code)
		if IsPreHandoffClientRequestIDConflict(err) {
			t.Fatalf("%s must not be pre-handoff conflict", code)
		}
		if !IsTerminalClientRequestIDConflict(err) {
			t.Fatalf("%s must be terminal conflict", code)
		}
		if !IsUnrecoverableClientRequestIDConflict(err) {
			t.Fatalf("%s must be unrecoverable", code)
		}
	}

	plain409 := fmt.Errorf("send: %w", &HTTPError{StatusCode: 409})
	if IsPreHandoffClientRequestIDConflict(plain409) || IsTerminalClientRequestIDConflict(plain409) || IsUnrecoverableClientRequestIDConflict(plain409) {
		t.Fatal("code-less 409 must not match any clientRequestId conflict predicate")
	}

	otherCode409 := conflictErr("SOMETHING_ELSE")
	if IsPreHandoffClientRequestIDConflict(otherCode409) || IsTerminalClientRequestIDConflict(otherCode409) || IsUnrecoverableClientRequestIDConflict(otherCode409) {
		t.Fatal("409 with unrelated code must not match any clientRequestId conflict predicate")
	}

	if IsPreHandoffClientRequestIDConflict(nil) || IsTerminalClientRequestIDConflict(nil) || IsUnrecoverableClientRequestIDConflict(nil) {
		t.Fatal("nil error must not match any predicate")
	}
}
