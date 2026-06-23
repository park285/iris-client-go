package client

import (
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
)

const (
	httpErrorBodyMaxLen      = 512
	httpErrorBodyDrainMaxLen = 64 << 10
)

type HTTPError struct {
	StatusCode int
	URL        string
	RetryAfter time.Duration
	// Body is a truncated (max 512 bytes), best-effort redacted snippet of the
	// response body intended for diagnostic logs. Callers should still treat it
	// as low-trust -- redaction covers common header echoes (Bearer, Authorization,
	// X-Iris-Secret/Token, X-API-Key, Cookie, Set-Cookie, Signature=) but is not
	// exhaustive. Do NOT forward Body to user-visible surfaces without re-review.
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
	return slog.GroupValue(
		slog.Int("StatusCode", e.StatusCode),
		slog.String("URL", e.URL),
		slog.String("Body", redactSensitiveTokens(e.Body)),
	)
}

// opInit은 transport 초기화 실패를 표시하는 TransportError.Op 값으로, ErrRetryable 분류에서 제외된다.
const opInit = "init"

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
	if r == nil {
		return ""
	}

	payload, _ := io.ReadAll(io.LimitReader(r, httpErrorBodyMaxLen))
	_, _ = io.CopyN(io.Discard, r, httpErrorBodyDrainMaxLen)

	return strings.TrimSpace(redactSensitiveTokens(string(payload)))
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
