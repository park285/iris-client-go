package iris

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	client "github.com/park285/iris-client-go/v3/internal/client/transport"
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
			err:               fmt.Errorf("publish reply: %w", replyHTTPError(t, http.StatusConflict, HTTPErrorCodeClientRequestIDFailed)),
			wantPreHandoff:    true,
			wantUnrecoverable: true,
		},
		{
			name:              "payload mismatch",
			err:               replyHTTPError(t, http.StatusConflict, HTTPErrorCodeClientRequestIDPayloadMismatch),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name:              "outcome unknown",
			err:               replyHTTPError(t, http.StatusConflict, HTTPErrorCodeClientRequestIDOutcomeUnknown),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name:              "already exists",
			err:               replyHTTPError(t, http.StatusConflict, HTTPErrorCodeClientRequestIDAlreadyExists),
			wantTerminal:      true,
			wantUnrecoverable: true,
		},
		{
			name: "wrong status with matching code",
			err:  replyHTTPError(t, http.StatusBadRequest, HTTPErrorCodeClientRequestIDFailed),
		},
		{
			name: "unknown conflict code",
			err:  replyHTTPError(t, http.StatusConflict, "UNKNOWN_CONFLICT"),
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

			if got := isTerminalClientRequestIDConflict(test.err); got != test.wantTerminal {
				t.Fatalf("isTerminalClientRequestIDConflict() = %v, want %v", got, test.wantTerminal)
			}

			if got := IsUnrecoverableClientRequestIDConflict(test.err); got != test.wantUnrecoverable {
				t.Fatalf("IsUnrecoverableClientRequestIDConflict() = %v, want %v", got, test.wantUnrecoverable)
			}
		})
	}
}

func replyHTTPError(t *testing.T, status int, code string) error {
	t.Helper()

	return replyHTTPResponseError(t, status, fmt.Sprintf(`{"code":%q}`, code))
}

func replyHTTPResponseError(t *testing.T, status int, body string) error {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write reply error: %v", err)
		}
	}))
	defer server.Close()

	api := NewAPIClient(server.URL, "synthetic-test-token", WithHTTPClient(server.Client()))

	defer func() {
		if err := api.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	err := api.SendMessage(t.Context(), "room", "test", WithClientRequestID("sdk-error-test"))
	if err == nil {
		t.Fatal("expected SDK HTTP error")
	}

	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != status {
		t.Fatalf("HTTP error = %v, want status %d", err, status)
	}

	return err
}

func TestReplyReissueUsesFullResponseCodeNotDiagnosticSnippet(t *testing.T) {
	t.Parallel()

	const failed = `{"code":"CLIENT_REQUEST_ID_FAILED"}`

	for _, tc := range []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "malformed_after_snippet", body: failed + strings.Repeat(" ", 512-len(failed)) + "!"},
		{name: "second_value_after_snippet", body: failed + strings.Repeat(" ", 512-len(failed)) + "{}"},
		{name: "duplicate_after_snippet", body: `{"code":"CLIENT_REQUEST_ID_FAILED","message":"` + strings.Repeat("x", 512) + `","code":"CLIENT_REQUEST_ID_OUTCOME_UNKNOWN"}`},
		{name: "valid_code_after_snippet", body: `{"message":"` + strings.Repeat("x", 512) + `","code":"CLIENT_REQUEST_ID_FAILED"}`, wantCode: HTTPErrorCodeClientRequestIDFailed},
		{name: "exact_parse_budget", body: failed + strings.Repeat(" ", (64<<10)-len(failed)), wantCode: HTTPErrorCodeClientRequestIDFailed},
		{name: "over_parse_budget", body: failed + strings.Repeat(" ", (64<<10)+1-len(failed))},
		{name: "malformed_beyond_parse_budget", body: failed + strings.Repeat(" ", (64<<10)-len(failed)) + "!"},
		{name: "truncated_json", body: `{"code":"CLIENT_REQUEST_ID_FAILED"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := fmt.Errorf("wrapped: %w", replyHTTPResponseError(t, http.StatusConflict, tc.body))
			if got := HTTPErrorCode(err); got != tc.wantCode {
				t.Fatalf("HTTPErrorCode() = %q, want %q", got, tc.wantCode)
			}

			assertClientRequestIDConflictPredicates(t, err, tc.wantCode != "", false, tc.wantCode != "")
		})
	}
}

func TestReplyReissueRejectsSynthesizedDiagnosticCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{
		HTTPErrorCodeClientRequestIDFailed,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDAlreadyExists,
	} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()

			err := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: http.StatusConflict, Body: fmt.Sprintf(`{"code":%q}`, code)})
			if got := HTTPErrorCode(err); got != "" {
				t.Fatalf("diagnostic code = %q, want empty", got)
			}

			assertClientRequestIDConflictPredicates(t, err, false, false, false)
		})
	}
}

func TestReplyReissueRejectsIncompleteHTTPBody(t *testing.T) {
	t.Parallel()

	const body = `{"code":"CLIENT_REQUEST_ID_FAILED"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)+1))
		w.WriteHeader(http.StatusConflict)

		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write incomplete HTTP body: %v", err)
		}
	}))
	defer server.Close()

	api := NewAPIClient(server.URL, "synthetic-test-token", WithHTTPClient(server.Client()))

	defer func() {
		if err := api.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	err := api.SendMessage(t.Context(), "room", "test", WithClientRequestID("sdk-read-error-test"))
	httpErr, ok := errors.AsType[*HTTPError](err)

	if !ok || httpErr.StatusCode != http.StatusConflict || httpErr.Body != body {
		t.Fatalf("HTTP diagnostic error = %v, want conflict with retained body", err)
	}

	if code := HTTPErrorCode(err); code != "" {
		t.Fatalf("incomplete HTTP body code = %q, want empty", code)
	}

	assertClientRequestIDConflictPredicates(t, err, false, false, false)
}
