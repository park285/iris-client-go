package iris

import (
	"fmt"
	"net/http"
	"testing"
)

func TestReplyReissueSuffixEnforcesGenerationBounds(t *testing.T) {
	t.Parallel()

	tests := map[int]string{
		-1:                             "",
		0:                              "",
		1:                              ":r1",
		ReplyReissueMaxGenerations:     ":r2",
		ReplyReissueMaxGenerations + 1: "",
	}
	for generation, want := range tests {
		t.Run(fmt.Sprintf("generation_%d", generation), func(t *testing.T) {
			t.Parallel()
			if got := replyReissueSuffix(generation); got != want {
				t.Fatalf("replyReissueSuffix(%d) = %q, want %q", generation, got, want)
			}
		})
	}
}

func TestReplyReissueConflictPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		err               error
		wantPreHandoff    bool
		wantTerminal      bool
		wantUnrecoverable bool
	}{
		{
			name:              "wrapped pre-handoff conflict",
			err:               fmt.Errorf("publish reply: %w", replyHTTPError(http.StatusConflict, HTTPErrorCodeClientRequestIDFailed)),
			wantPreHandoff:    true,
			wantUnrecoverable: true,
		},
		{
			name:              "payload mismatch",
			err:               replyHTTPError(http.StatusConflict, HTTPErrorCodeClientRequestIDPayloadMismatch),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name:              "outcome unknown",
			err:               replyHTTPError(http.StatusConflict, HTTPErrorCodeClientRequestIDOutcomeUnknown),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name:              "already exists",
			err:               replyHTTPError(http.StatusConflict, HTTPErrorCodeClientRequestIDAlreadyExists),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name: "wrong status with matching code",
			err:  replyHTTPError(http.StatusBadRequest, HTTPErrorCodeClientRequestIDFailed),
		},
		{
			name: "unknown conflict code",
			err:  replyHTTPError(http.StatusConflict, "UNKNOWN_CONFLICT"),
		},
		{
			name: "empty conflict body",
			err:  &HTTPError{StatusCode: http.StatusConflict},
		},
		{
			name: "non-http error",
			err:  fmt.Errorf("network unavailable"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPreHandoffClientRequestIDConflict(test.err); got != test.wantPreHandoff {
				t.Fatalf("IsPreHandoffClientRequestIDConflict() = %v, want %v", got, test.wantPreHandoff)
			}
			if got := IsTerminalClientRequestIDConflict(test.err); got != test.wantTerminal {
				t.Fatalf("IsTerminalClientRequestIDConflict() = %v, want %v", got, test.wantTerminal)
			}
			if got := IsUnrecoverableClientRequestIDConflict(test.err); got != test.wantUnrecoverable {
				t.Fatalf("IsUnrecoverableClientRequestIDConflict() = %v, want %v", got, test.wantUnrecoverable)
			}
		})
	}
}

func replyHTTPError(status int, code string) *HTTPError {
	return &HTTPError{
		StatusCode: status,
		Body:       fmt.Sprintf(`{"code":%q}`, code),
	}
}
