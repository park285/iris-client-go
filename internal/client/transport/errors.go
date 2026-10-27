package transport

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var (
	ErrRetryable      = errors.New("iris: retryable error")
	ErrPermanent      = errors.New("iris: permanent error")
	ErrAuthFailed     = errors.New("iris: authentication failed")
	ErrRateLimited    = errors.New("iris: rate limited")
	ErrTransport      = errors.New("iris: transport error")
	ErrH3EgressDenied = errors.New("iris: H3 egress denied")
	// ErrNotSent는 요청이 Iris로 나가기 전에 실패가 확정됐음을 뜻한다(egress 거부, dial 전 resolve
	// 실패·취소, http1 probe 전용 transport의 보호 route). 부수효과가 없었음이 확정이라 ErrTransport
	// (결과 불명)와 함께 성립하지 않는다.
	ErrNotSent = errors.New("iris: request not sent")
)

// Iris reply admission이 clientRequestId 기록을 조회한 뒤 409와 함께 돌려주는 machine-readable
// code다. 이 code는 그 id의 admission 판정이므로 앞선 attempt의 결과가 불명이어도 그대로 전달한다.
// 이 리터럴이 정본이며 iris 패키지는 alias만 둔다. Iris-stack의 check-stack-reissue-contract가 이
// 파일의 리터럴을 Iris와 대조한다.
const (
	HTTPErrorCodeClientRequestIDPayloadMismatch = "CLIENT_REQUEST_ID_PAYLOAD_MISMATCH"
	HTTPErrorCodeClientRequestIDFailed          = "CLIENT_REQUEST_ID_FAILED"
	HTTPErrorCodeClientRequestIDOutcomeUnknown  = "CLIENT_REQUEST_ID_OUTCOME_UNKNOWN"
	HTTPErrorCodeClientRequestIDAlreadyExists   = "CLIENT_REQUEST_ID_ALREADY_EXISTS"
)

const (
	httpErrorBodyMaxLen      = 512
	httpErrorBodyParseMaxLen = 64 << 10
	httpErrorBodyDrainMaxLen = httpErrorBodyMaxLen
	httpErrorCodeMaxLen      = 128
)

type HTTPError struct {
	StatusCode int
	URL        string
	RetryAfter time.Duration
	// Body는 진단 로그용으로 최대 512바이트까지 잘린, best-effort로 민감정보를 가린
	// 응답 본문 스니펫이다. 그래도 호출자는 이 값을 low-trust로 취급해야 한다. redaction은
	// 흔한 헤더 반향(Bearer, Authorization, X-Iris-Secret/Token, X-API-Key, Cookie,
	// Set-Cookie, Signature=)을 가리지만 완전하지 않다. 재검토 없이 Body를 사용자에게
	// 노출되는 표면으로 전달하지 마라.
	Body string
}

func (e *HTTPError) Error() string {
	target := strings.TrimSpace(e.URL)
	if target == "" {
		target = "request"
	}

	if body := redactSensitiveTokens(e.Body); body != "" {
		return fmt.Sprintf("iris %s returned %d: %s", target, e.StatusCode, body)
	}

	return fmt.Sprintf("iris %s returned %d", target, e.StatusCode)
}

func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrRetryable:
		return e.StatusCode >= 500 || e.StatusCode == httpStatusTooManyRequests
	case ErrPermanent:
		return e.StatusCode >= 400 && e.StatusCode < 500 && e.StatusCode != httpStatusTooManyRequests
	case ErrAuthFailed:
		return e.StatusCode == httpStatusUnauthorized || e.StatusCode == httpStatusForbidden
	case ErrRateLimited:
		return e.StatusCode == httpStatusTooManyRequests
	default:
		return false
	}
}

func (e *HTTPError) LogValue() slog.Value {
	return e.logValue("")
}

func (e *HTTPError) logValue(code string) slog.Value {
	return slog.GroupValue(
		slog.Int("StatusCode", e.StatusCode),
		slog.String("URL", e.URL),
		slog.String("Code", code),
		slog.String("Body", redactSensitiveTokens(e.Body)),
	)
}

type codedHTTPError struct {
	httpErr *HTTPError
	code    string
}

type httpCodedError interface {
	error
	httpErrorCode() string
}

func (e *codedHTTPError) Error() string {
	return e.httpErr.Error()
}

func (e *codedHTTPError) Unwrap() error {
	return e.httpErr
}

func (e *codedHTTPError) LogValue() slog.Value {
	return e.httpErr.logValue(e.code)
}

func (e *codedHTTPError) httpErrorCode() string {
	return e.code
}

func withHTTPErrorCode(httpErr *HTTPError, code string) error {
	if code == "" {
		return httpErr
	}

	return &codedHTTPError{httpErr: httpErr, code: code}
}

// HTTPErrorCode는 Iris HTTP error chain에 보존된 machine-readable code를 반환한다.
// Code가 없거나 token 계약을 통과하지 못한 응답이면 빈 문자열을 반환한다.
// HTTPError.Body는 잘리거나 redaction된 진단값이므로 code의 원본으로 읽지 않는다.
func HTTPErrorCode(err error) string {
	if coded, ok := errors.AsType[httpCodedError](err); ok {
		return coded.httpErrorCode()
	}

	return ""
}

// ClientRequestIDConflictCode는 409 응답의 CLIENT_REQUEST_ID_* code를 반환한다. 409가 아니거나 네
// code 중 하나가 아니면 빈 문자열이다. 재시도 결과 불명 해소와 iris 재발급 판정이 이 함수 하나를 쓴다.
func ClientRequestIDConflictCode(err error) string {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr == nil || httpErr.StatusCode != httpStatusConflict {
		return ""
	}

	switch code := HTTPErrorCode(err); code {
	case HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDFailed,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDAlreadyExists:
		return code
	default:
		return ""
	}
}

const opRetryWait = "retry wait"

// opRetryOutcomeUnknown은 결과 불명 attempt 뒤 재시도가 그 결과를 판정하지 못하고 끝났음을 표시한다.
const opRetryOutcomeUnknown = "retry outcome unknown"

// TransportError는 요청이 Iris에 도달해 처리됐는지 알 수 없는 전송 실패다. Err가 ErrNotSent를 감싸면
// 전송 전 실패가 확정된 것이라 ErrTransport로 분류하지 않는다.
type TransportError struct {
	Op  string
	URL string
	Err error
	// Idempotent는 같은 요청을 다시 보내도 부수효과가 중복되지 않는지다. GET·조회 POST는 참이고,
	// reply POST는 clientRequestId가 있을 때만 참이다. 거짓이면 결과 불명 실패를 ErrRetryable로
	// 분류하지 않는다.
	Idempotent bool
}

func (e *TransportError) Error() string {
	if e == nil {
		return "<nil>"
	}

	prefix := strings.TrimSpace(strings.TrimSpace(e.Op) + " " + redactedURLForError(e.URL))
	if prefix == "" {
		prefix = "transport"
	}

	if e.Err == nil {
		return "iris transport " + prefix
	}

	return fmt.Sprintf("iris transport %s: %v", prefix, e.Err)
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// Is는 전송 전 확정 실패를 결과 불명(ErrTransport)에서 빼고, 재시도는 부수효과가 중복되지 않는
// 경우(전송 전 실패 또는 Idempotent)만 ErrRetryable로 분류한다. Egress 거부와 상한 초과는 다시
// 보내도 같은 결과라 ErrRetryable이 아니다.
func (e *TransportError) Is(target error) bool {
	notSent := errors.Is(e.Err, ErrNotSent)

	switch target {
	case ErrTransport:
		return !notSent
	case ErrRetryable:
		return (notSent || e.Idempotent) && !errors.Is(e.Err, ErrH3EgressDenied) && !errors.Is(e.Err, ErrResponseTooLarge)
	default:
		return false
	}
}

func redactedURLForError(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "" {
		return ""
	}

	if u, err := url.Parse(target); err == nil {
		u.User = nil
		u.RawQuery = ""
		u.ForceQuery = false
		u.Fragment = ""

		if s := strings.TrimSpace(u.String()); s != "" {
			return s
		}
	}

	if strings.ContainsAny(target, "?#@") {
		return "request"
	}

	return target
}

// complete는 전체 본문이 parse 예산 안에서 읽기 오류 없이 EOF에 도달했음을 뜻한다.
func readErrorBody(r io.Reader) (payload []byte, complete bool) {
	if r == nil {
		return nil, false
	}

	payload, err := io.ReadAll(io.LimitReader(r, httpErrorBodyParseMaxLen+1))
	if err != nil {
		return payload, false
	}

	drainBounded(r, httpErrorBodyDrainMaxLen)

	return payload, len(payload) <= httpErrorBodyParseMaxLen
}

func truncateErrorBody(payload []byte) string {
	if len(payload) > httpErrorBodyMaxLen {
		payload = payload[:httpErrorBodyMaxLen]
	}

	return strings.TrimSpace(redactSensitiveTokens(string(payload)))
}

func parseHTTPErrorCode(body string) string {
	var payload struct {
		Code string `json:"code"`
	}

	if err := jsonv2.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}

	code := strings.TrimSpace(payload.Code)
	if code == "" || len(code) > httpErrorCodeMaxLen {
		return ""
	}

	for i := range len(code) {
		char := code[i]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}

		return ""
	}

	return code
}

func redactSensitiveTokens(s string) string {
	for _, prefix := range []string{
		"authorization:",
		"x-iris-secret:",
		"x-iris-token:",
		"x-api-key:",
		"cookie:",
		"set-cookie:",
	} {
		s = redactPrefix(s, prefix, true)
	}

	for _, prefix := range []string{"bearer ", "signature="} {
		s = redactPrefix(s, prefix, false)
	}

	return s
}

func redactPrefix(s, prefix string, redactLine bool) string {
	lower := strings.ToLower(s)
	searchFrom := 0

	for {
		idx := strings.Index(lower[searchFrom:], prefix)
		if idx < 0 {
			return s
		}

		idx += searchFrom

		restStart := idx + len(prefix)
		rest := s[restStart:]
		valueStart := strings.IndexFunc(rest, func(r rune) bool {
			return !unicode.IsSpace(r)
		})

		if valueStart < 0 {
			return s
		}

		valueStart += restStart

		value := s[valueStart:]
		valueEnd := strings.IndexFunc(value, func(r rune) bool {
			if redactLine {
				return r == '\r' || r == '\n'
			}

			return unicode.IsSpace(r) || r == ',' || r == ';' || r == '"' || r == '\''
		})

		if valueEnd < 0 {
			valueEnd = len(s)
		} else {
			valueEnd += valueStart
		}

		s = s[:valueStart] + "***" + s[valueEnd:]
		lower = strings.ToLower(s)
		searchFrom = min(valueStart+len("***"), len(lower))
	}
}

const (
	httpStatusUnauthorized    = 401
	httpStatusForbidden       = 403
	httpStatusConflict        = 409
	httpStatusTooManyRequests = 429
)
