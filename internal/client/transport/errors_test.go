package transport

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestErrRetryable_MatchesWrappedHTTPError(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 503, URL: testReplyURL}

	var err error = httpErr

	if !errors.Is(err, ErrRetryable) {
		t.Fatal("expected errors.Is(err, ErrRetryable) to be true, got false")
	}

	got, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatal("expected errors.AsType to extract *HTTPError, failed")
	}

	if got.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("StatusCode=%d, want 503", got.StatusCode)
	}
}

func TestErrPermanent_DoesNotMatchRetryable(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 400, URL: testReplyURL}

	var err error = httpErr

	if errors.Is(err, ErrRetryable) {
		t.Fatal("400 must not be retryable")
	}

	if !errors.Is(err, ErrPermanent) {
		t.Fatal("400 must match ErrPermanent")
	}
}

func TestErrAuthFailed_Matches401(t *testing.T) {
	var err error = &HTTPError{StatusCode: 401, URL: testReplyURL}

	if !errors.Is(err, ErrAuthFailed) {
		t.Fatal("401 must match ErrAuthFailed")
	}
}

func TestErrRateLimited_Matches429(t *testing.T) {
	var err error = &HTTPError{StatusCode: 429, URL: testReplyURL}

	if !errors.Is(err, ErrRateLimited) {
		t.Fatal("429 must match ErrRateLimited")
	}

	if !errors.Is(err, ErrRetryable) {
		t.Fatal("429 must also match ErrRetryable (retry with backoff)")
	}
}

// TransportError 분류는 결과 불명(ErrTransport)과 전송 전 확정 실패(ErrNotSent)를 나누고, 부수효과가
// 중복될 수 있는 결과 불명 실패를 ErrRetryable로 권하지 않는다.
func TestTransportErrorClassification(t *testing.T) {
	t.Parallel()

	notSentResolve := fmt.Errorf("%w: resolve h3 dial addr: %w", ErrNotSent, errors.New("no such host"))
	notSentEgress := fmt.Errorf("%w: %w: %w", ErrNotSent, ErrH3EgressDenied, errors.New("policy"))

	tests := []struct {
		name          string
		err           *TransportError
		wantTransport bool
		wantNotSent   bool
		wantRetryable bool
	}{
		{name: "idempotent timeout", err: &TransportError{Err: errors.New("timeout"), Idempotent: true}, wantTransport: true, wantRetryable: true},
		{name: "reply without clientRequestId timeout", err: &TransportError{Err: errors.New("timeout")}, wantTransport: true},
		{name: "resolve before dial", err: &TransportError{Err: notSentResolve}, wantNotSent: true, wantRetryable: true},
		{name: "egress denied", err: &TransportError{Err: notSentEgress, Idempotent: true}, wantNotSent: true},
		{name: "response too large", err: &TransportError{Err: ErrResponseTooLarge, Idempotent: true}, wantTransport: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := errors.Is(test.err, ErrTransport); got != test.wantTransport {
				t.Errorf("errors.Is(ErrTransport) = %v, want %v", got, test.wantTransport)
			}

			if got := errors.Is(test.err, ErrNotSent); got != test.wantNotSent {
				t.Errorf("errors.Is(ErrNotSent) = %v, want %v", got, test.wantNotSent)
			}

			if got := errors.Is(test.err, ErrRetryable); got != test.wantRetryable {
				t.Errorf("errors.Is(ErrRetryable) = %v, want %v", got, test.wantRetryable)
			}
		})
	}
}

func TestTransportError_ErrorRedactsURLSecrets(t *testing.T) {
	te := &TransportError{
		Op: "post",
		// #nosec G101 -- URL 비밀 삭제를 검증하려면 이 픽스처에 비밀이 들어 있어야 한다.
		URL: "https://user:secret@iris.test/reply?token=abc123&room=42#frag",
		Err: errors.New("connection refused"),
	}

	got := te.Error()

	for _, forbidden := range []string{"user", "secret", "token=", "abc123", "room=42", "#frag"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("TransportError leaked %q in %q", forbidden, got)
		}
	}

	if !strings.Contains(got, "https://iris.test/reply") {
		t.Fatalf("TransportError = %q, want redacted target path", got)
	}
}

func TestReadTruncatedErrorBody_RedactsBearerToken(t *testing.T) {
	in := strings.NewReader("error context Bearer abcdef1234567890 trailing")
	got := readTruncatedErrorBody(in)

	if strings.Contains(got, "abcdef1234567890") {
		t.Fatalf("token leaked: %q", got)
	}
}

func TestRedactSensitiveTokens_RedactsXIrisSecretValue(t *testing.T) {
	in := "error context X-Iris-Secret: abcdef1234567890 trailing"
	got := redactSensitiveTokens(in)

	if strings.Contains(got, "abcdef1234567890") {
		t.Fatalf("X-Iris-Secret value leaked: %q", got)
	}
}

func TestRedactSensitiveTokens_RedactsXIrisSecretWhitespaceVariants(t *testing.T) {
	for _, in := range []string{
		"error context X-Iris-Secret:\tabc123 trailing",
		"error context X-Iris-Secret:abc123 trailing",
	} {
		got := redactSensitiveTokens(in)
		if strings.Contains(got, "abc123") {
			t.Fatalf("X-Iris-Secret value leaked for %q: %q", in, got)
		}
	}
}

func TestRedactSensitiveTokens_CaseInsensitive(t *testing.T) {
	cases := []struct{ in, mustNotContain string }{
		{"authorization: bearer abc123", "abc123"},
		{"Authorization: Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"X-Iris-Token: token-value-xyz", "token-value-xyz"},
		{"X-API-Key: key-12345", "key-12345"},
		{"Cookie: session=secret-cookie", "secret-cookie"},
		{"Set-Cookie: id=abc; HttpOnly", "abc"},
	}
	for _, tc := range cases {
		got := redactSensitiveTokens(tc.in)
		if strings.Contains(got, tc.mustNotContain) {
			t.Fatalf("leaked %q in %q: %q", tc.mustNotContain, tc.in, got)
		}
	}
}

func TestReadTruncatedErrorBody_Caps512Bytes(t *testing.T) {
	in := strings.NewReader(strings.Repeat("x", 2000))
	got := readTruncatedErrorBody(in)

	if len(got) > 512 {
		t.Fatalf("body length %d > 512", len(got))
	}
}

func TestReadErrorResponse_ExtractsStructuredCodeAndPreservesBody(t *testing.T) {
	body := `{"message":"request failed","code":"CLIENT_REQUEST_ID_FAILED"}`
	resp := &http.Response{
		StatusCode: http.StatusConflict,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := readErrorResponse("/reply", resp)
	if got := HTTPErrorCode(err); got != "CLIENT_REQUEST_ID_FAILED" {
		t.Fatalf("HTTPErrorCode() = %q, want CLIENT_REQUEST_ID_FAILED", got)
	}

	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatal("readErrorResponse() did not preserve *HTTPError")
	}

	if httpErr.Body != body {
		t.Fatalf("Body = %q, want %q", httpErr.Body, body)
	}

	if httpErr.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %s, want zero", httpErr.RetryAfter)
	}
}

func TestReadErrorResponse_ExtractsCodeBeforeBodyTruncation(t *testing.T) {
	body := `{"message":"` + strings.Repeat("x", httpErrorBodyMaxLen+128) + `","code":"CLIENT_REQUEST_ID_OUTCOME_UNKNOWN"}`
	resp := &http.Response{
		StatusCode: http.StatusConflict,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := readErrorResponse("/reply", resp)
	if got := HTTPErrorCode(err); got != "CLIENT_REQUEST_ID_OUTCOME_UNKNOWN" {
		t.Fatalf("HTTPErrorCode() = %q, want CLIENT_REQUEST_ID_OUTCOME_UNKNOWN", got)
	}

	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatal("readErrorResponse() did not preserve *HTTPError")
	}

	if len(httpErr.Body) > httpErrorBodyMaxLen {
		t.Fatalf("Body length = %d, want <= %d", len(httpErr.Body), httpErrorBodyMaxLen)
	}
}

func TestReadErrorResponse_ExtractsCodeBeforeBodyRedaction(t *testing.T) {
	body := `{"message":"Authorization: private-value","code":"CLIENT_REQUEST_ID_FAILED"}`
	resp := &http.Response{
		StatusCode: http.StatusConflict,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := fmt.Errorf("wrapped: %w", readErrorResponse("/reply", resp))
	if got := HTTPErrorCode(err); got != "CLIENT_REQUEST_ID_FAILED" {
		t.Fatalf("HTTPErrorCode() = %q, want CLIENT_REQUEST_ID_FAILED", got)
	}

	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatal("wrapped error did not preserve *HTTPError")
	}

	if strings.Contains(httpErr.Body, "private-value") {
		t.Fatalf("Body leaked redacted value: %q", httpErr.Body)
	}
}

func TestHTTPErrorCodeReturnsEmptyForPlainHTTPError(t *testing.T) {
	err := &HTTPError{StatusCode: http.StatusConflict}
	if got := HTTPErrorCode(err); got != "" {
		t.Fatalf("HTTPErrorCode() = %q, want empty", got)
	}
}

func TestHTTPErrorCodeIgnoresDiagnosticHTTPErrorBody(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &HTTPError{
		StatusCode: http.StatusConflict,
		Body:       `{"code":"CLIENT_REQUEST_ID_PAYLOAD_MISMATCH"}`,
	})
	if got := HTTPErrorCode(err); got != "" {
		t.Fatalf("HTTPErrorCode() = %q, want no code from diagnostic Body", got)
	}
}

func TestParseHTTPErrorCode_RejectsLowTrustValues(t *testing.T) {
	// #nosec G101 -- HTTP 에러 코드 파싱 입력이며 자격증명이 아니다.
	tests := map[string]string{
		"malformed":        `{"code":`,
		"non-json":         `CLIENT_REQUEST_ID_FAILED`,
		"missing":          `{"message":"failed"}`,
		"non-string":       `{"code":42}`,
		"invalid token":    `{"code":"FAILED with details"}`,
		"oversized":        `{"code":"` + strings.Repeat("A", httpErrorCodeMaxLen+1) + `"}`,
		"truncated object": `{"code":"CLIENT_REQUEST_ID_FAILED"`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if got := parseHTTPErrorCode(body); got != "" {
				t.Fatalf("parseHTTPErrorCode() = %q, want empty", got)
			}
		})
	}
}

// readTruncatedErrorBody는 오류 응답 경로가 쓰는 readErrorBody → truncateErrorBody 조합을 그대로 재현한다.
func readTruncatedErrorBody(r io.Reader) string {
	payload, _ := readErrorBody(r)

	return truncateErrorBody(payload)
}

// clientRequestId 없는 reply의 전송 실패는 결과 불명이라 ErrRetryable이 아니다. 같은 id로 다시 보낼 수
// 있는 reply와 조회 GET만 ErrRetryable이다.
func TestTransportFailureRetryableFollowsIdempotency(t *testing.T) {
	t.Parallel()

	client := mustNewAPIClient("http://localhost", "token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})))

	_, withoutID := client.SendMessageAccepted(t.Context(), testRoom, "hello")
	if !errors.Is(withoutID, ErrTransport) || errors.Is(withoutID, ErrRetryable) {
		t.Fatalf("reply without clientRequestId error = %v, want ErrTransport and not ErrRetryable", withoutID)
	}

	_, withID := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("client:retry:0001"))
	if !errors.Is(withID, ErrTransport) || !errors.Is(withID, ErrRetryable) {
		t.Fatalf("reply with clientRequestId error = %v, want ErrTransport and ErrRetryable", withID)
	}

	_, getErr := client.GetRooms(t.Context())
	if !errors.Is(getErr, ErrTransport) || !errors.Is(getErr, ErrRetryable) {
		t.Fatalf("GET error = %v, want ErrTransport and ErrRetryable", getErr)
	}
}
