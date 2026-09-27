package iris

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	client "github.com/park285/iris-client-go/v2/internal/client/transport"
)

// transport 재시도 루프는 이 code들을 clientRequestId admission 판정으로 보고 결과 불명 분류에서
// 제외한다. 공개 상수와 transport 사본이 갈라지면 판정이 조용히 틀어지므로 값을 고정한다.
func TestClientRequestIDConflictCodesMatchTransport(t *testing.T) {
	t.Parallel()

	pairs := map[string][2]string{
		"PayloadMismatch": {HTTPErrorCodeClientRequestIDPayloadMismatch, client.HTTPErrorCodeClientRequestIDPayloadMismatch},
		"Failed":          {HTTPErrorCodeClientRequestIDFailed, client.HTTPErrorCodeClientRequestIDFailed},
		"OutcomeUnknown":  {HTTPErrorCodeClientRequestIDOutcomeUnknown, client.HTTPErrorCodeClientRequestIDOutcomeUnknown},
		"AlreadyExists":   {HTTPErrorCodeClientRequestIDAlreadyExists, client.HTTPErrorCodeClientRequestIDAlreadyExists},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s: public code %q != transport code %q", name, pair[0], pair[1])
		}
	}
}

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
			err:  errors.New("network unavailable"),
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
