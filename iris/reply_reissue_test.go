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
	if got := replyReissueSuffix(0); got != "" {
		t.Fatalf("replyReissueSuffix(0) = %q, want empty", got)
	}

	if got := replyReissueSuffix(-1); got != "" {
		t.Fatalf("replyReissueSuffix(-1) = %q, want empty", got)
	}

	if got := replyReissueSuffix(1); got != ":r1" {
		t.Fatalf("replyReissueSuffix(1) = %q, want :r1", got)
	}

	if got := replyReissueSuffix(2); got != ":r2" {
		t.Fatalf("replyReissueSuffix(2) = %q, want :r2", got)
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

	if _, reissueErr := ReissuedClientRequestID("chatbotgo:abc123", 0); !errors.Is(reissueErr, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation 0 error = %v, want ErrReplyReissueGenerationOutOfRange", reissueErr)
	}

	if _, reissueErr := ReissuedClientRequestID("chatbotgo:abc123", -1); !errors.Is(reissueErr, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation -1 error = %v, want ErrReplyReissueGenerationOutOfRange", reissueErr)
	}

	if _, reissueErr := ReissuedClientRequestID("chatbotgo:abc123", ReplyReissueMaxGenerations+1); !errors.Is(reissueErr, ErrReplyReissueGenerationOutOfRange) {
		t.Fatalf("generation over max error = %v, want ErrReplyReissueGenerationOutOfRange", reissueErr)
	}

	if _, reissueErr := ReissuedClientRequestID("", 1); reissueErr == nil {
		t.Fatal("empty base error = nil, want error")
	}

	if _, reissueErr := ReissuedClientRequestID(strings.Repeat("a", 159), 1); reissueErr == nil {
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
	t.Run(HTTPErrorCodeClientRequestIDFailed, func(t *testing.T) {
		assertClientRequestIDConflictPredicates(t, conflictErr(HTTPErrorCodeClientRequestIDFailed), true, false, true)
	})

	for _, code := range []string{
		HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDAlreadyExists,
	} {
		t.Run(code, func(t *testing.T) {
			assertClientRequestIDConflictPredicates(t, conflictErr(code), false, true, true)
		})
	}

	nonConflicts := []struct {
		name string
		err  error
	}{
		{name: "code-less 409", err: fmt.Errorf("send: %w", &HTTPError{StatusCode: 409})},
		{name: "409 with unrelated code", err: conflictErr("SOMETHING_ELSE")},
		{name: "non-409 status with CLIENT_REQUEST_ID_FAILED code", err: fmt.Errorf("send: %w", &HTTPError{StatusCode: 500, Body: fmt.Sprintf(`{"code":%q}`, HTTPErrorCodeClientRequestIDFailed)})},
		{name: "nil error"},
	}

	for _, tc := range nonConflicts {
		t.Run(tc.name, func(t *testing.T) {
			assertClientRequestIDConflictPredicates(t, tc.err, false, false, false)
		})
	}
}

func assertClientRequestIDConflictPredicates(tb testing.TB, err error, wantPreHandoff, wantTerminal, wantUnrecoverable bool) {
	tb.Helper()

	if got := IsPreHandoffClientRequestIDConflict(err); got != wantPreHandoff {
		tb.Fatalf("IsPreHandoffClientRequestIDConflict(%v) = %t, want %t", err, got, wantPreHandoff)
	}

	if got := IsTerminalClientRequestIDConflict(err); got != wantTerminal {
		tb.Fatalf("IsTerminalClientRequestIDConflict(%v) = %t, want %t", err, got, wantTerminal)
	}

	if got := IsUnrecoverableClientRequestIDConflict(err); got != wantUnrecoverable {
		tb.Fatalf("IsUnrecoverableClientRequestIDConflict(%v) = %t, want %t", err, got, wantUnrecoverable)
	}
}
