package transport

import (
	"encoding/json"
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

	ErrCertReloadTokenRequired = errors.New("iris: cert-reload requires a dedicated cert-reload token; set WithCertReloadToken")
	ErrInboundSecretRequired   = errors.New("iris: /config* (inbound) route signing requires an inbound secret; set WithInboundSecret or WithHMACSecret (the bot token is not used for inbound signing)")
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
// code가 없거나 token 계약을 통과하지 못한 응답이면 빈 문자열을 반환한다.
func HTTPErrorCode(err error) string {
	var coded interface{ httpErrorCode() string }
	if errors.As(err, &coded) {
		return coded.httpErrorCode()
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return parseHTTPErrorCode(httpErr.Body)
	}

	return ""
}

// opInit은 transport 초기화 실패를 표시하는 TransportError.Op 값으로, ErrRetryable 분류에서 제외된다.
const opInit = "init"

const opRetryWait = "retry wait"

type TransportError struct {
	Op  string
	URL string
	Err error
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

func (e *TransportError) Is(target error) bool {
	switch target {
	case ErrTransport:
		return true
	case ErrRetryable:
		return e.Op != opInit && !errors.Is(e.Err, ErrH3EgressDenied)
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

type PingError struct {
	URL    string
	Reason string
	Err    error
}

func (e *PingError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if strings.TrimSpace(e.URL) != "" && strings.TrimSpace(e.Reason) != "" {
		return fmt.Sprintf("iris ping %s: %s", e.URL, e.Reason)
	}
	if strings.TrimSpace(e.Reason) != "" {
		return "iris ping: " + e.Reason
	}
	if err := e.Unwrap(); err != nil {
		return err.Error()
	}
	return "iris ping failed"
}

func (e *PingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *PingError) Is(target error) bool {
	return target == ErrPermanent
}

func truncateBody(r io.Reader) string {
	return truncateErrorBody(readErrorBody(r))
}

func readErrorBody(r io.Reader) []byte {
	if r == nil {
		return nil
	}

	payload, _ := io.ReadAll(io.LimitReader(r, httpErrorBodyParseMaxLen))
	_, _ = io.CopyN(io.Discard, r, httpErrorBodyDrainMaxLen)

	return payload
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
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
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
	httpStatusTooManyRequests = 429
)
