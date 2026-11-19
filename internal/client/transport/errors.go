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
	ErrNotSent        = errors.New("iris: request not sent")
)

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
	Body       string
	code       string
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
	if e.StatusCode == httpStatusUnauthorized && e.code == authEpochMismatchCode {
		return target == ErrRetryable || target == ErrNotSent
	}

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
		slog.String("Code", e.code),
		slog.String("Body", redactSensitiveTokens(e.Body)),
	)
}

func HTTPErrorCode(err error) string {
	if httpErr, ok := errors.AsType[*HTTPError](err); ok && httpErr != nil {
		return httpErr.code
	}

	return ""
}

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

const opRetryOutcomeUnknown = "retry outcome unknown"

var errReplyStatusIdentityMismatch = errors.New("reply status response identifies a different request")

type TransportError struct {
	Op         string
	URL        string
	Err        error
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

func (e *TransportError) Is(target error) bool {
	if e == nil {
		return false
	}

	notSent := errors.Is(e.Err, ErrNotSent)

	switch target {
	case ErrTransport:
		return !notSent
	case ErrPermanent:
		return errors.Is(e.Err, errReplyStatusIdentityMismatch)
	case ErrRetryable:
		return (notSent || e.Idempotent) && !errors.Is(e.Err, ErrH3EgressDenied) && !errors.Is(e.Err, ErrResponseTooLarge) && !errors.Is(e.Err, errReplyStatusIdentityMismatch)
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
	if code == authEpochMismatchCode && code != payload.Code {
		return ""
	}

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
