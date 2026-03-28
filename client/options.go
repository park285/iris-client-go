package client

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"unicode"
)

type SendOption func(*sendOptions)

type sendOptions struct {
	ThreadID    *string
	ThreadScope *int
}

func WithThreadID(id string) SendOption {
	return func(o *sendOptions) {
		o.ThreadID = &id
	}
}

func WithThreadScope(scope int) SendOption {
	return func(o *sendOptions) {
		o.ThreadScope = &scope
	}
}

func applySendOptions(opts []SendOption) sendOptions {
	var result sendOptions
	for _, opt := range opts {
		opt(&result)
	}
	return result
}

func validateSendOptions(o sendOptions) error {
	if o.ThreadID != nil {
		for _, r := range *o.ThreadID {
			if !unicode.IsDigit(r) {
				return fmt.Errorf("iris: threadId must be numeric, got %q", *o.ThreadID)
			}
		}
	}

	if o.ThreadScope != nil && *o.ThreadScope <= 0 {
		return fmt.Errorf("iris: threadScope must be positive, got %d", *o.ThreadScope)
	}

	if o.ThreadScope != nil && *o.ThreadScope >= 2 && o.ThreadID == nil {
		return errors.New("iris: threadScope >= 2 requires threadId")
	}

	return nil
}

type clientOptions struct {
	Transport             string
	Timeout               time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	ReadIdleTimeout       time.Duration
	PingTimeout           time.Duration
	PingProbeTimeout      time.Duration
	PingStrategy          PingStrategy
	WriteByteTimeout      time.Duration
	Logger                *slog.Logger
	HTTPClient            *http.Client
	RoundTripper          http.RoundTripper
	ReplyRetryMax         int // 0 = disabled (default), >0 = max attempts for 429 retry
	hmacSecret            string
	baseURL               string
	botToken              string
}

type ClientOption func(*clientOptions)

type PingStrategy int

const (
	PingStrategyAuto PingStrategy = iota // default: /ready -> /health -> OPTIONS /reply with fallback
	PingStrategyReady
	PingStrategyHealth
)

func WithTransport(transport string) ClientOption {
	return func(o *clientOptions) {
		o.Transport = transport
	}
}

func WithTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.Timeout = d
	}
}

func WithDialTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.DialTimeout = d
	}
}

func WithTLSHandshakeTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.TLSHandshakeTimeout = d
	}
}

func WithResponseHeaderTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.ResponseHeaderTimeout = d
	}
}

func WithIdleConnTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.IdleConnTimeout = d
	}
}

func WithMaxIdleConns(n int) ClientOption {
	return func(o *clientOptions) {
		o.MaxIdleConns = n
	}
}

func WithMaxIdleConnsPerHost(n int) ClientOption {
	return func(o *clientOptions) {
		o.MaxIdleConnsPerHost = n
	}
}

func WithMaxConnsPerHost(n int) ClientOption {
	return func(o *clientOptions) {
		o.MaxConnsPerHost = n
	}
}

func WithReadIdleTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.ReadIdleTimeout = d
	}
}

func WithPingTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.PingTimeout = d
	}
}

func WithPingProbeTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.PingProbeTimeout = d
	}
}

func WithPingStrategy(s PingStrategy) ClientOption {
	return func(o *clientOptions) {
		o.PingStrategy = s
	}
}

func WithWriteByteTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.WriteByteTimeout = d
	}
}

func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.Logger = logger
	}
}

func WithHTTPClient(c *http.Client) ClientOption {
	return func(o *clientOptions) {
		if c != nil {
			o.HTTPClient = c
		}
	}
}

func WithRoundTripper(rt http.RoundTripper) ClientOption {
	return func(o *clientOptions) {
		if rt != nil {
			o.RoundTripper = rt
		}
	}
}

// WithReplyRetry는 reply 경로에서만 HTTP 429를 재시도합니다.
// Iris 서버 계약상 429만 미처리 응답으로 간주할 수 있으므로 다른 오류는 재시도하지 않습니다.
func WithReplyRetry(maxAttempts int) ClientOption {
	return func(o *clientOptions) {
		o.ReplyRetryMax = maxAttempts
	}
}

// WithHMACSecret는 지정한 비밀키로 HMAC-SHA256 요청 서명을 활성화합니다.
// 설정하면 bot token 대신 이 값을 요청 서명 비밀키로 사용합니다.
func WithHMACSecret(secret string) ClientOption {
	return func(o *clientOptions) {
		o.hmacSecret = secret
	}
}

func WithBaseURL(url string) ClientOption {
	return func(o *clientOptions) {
		o.baseURL = url
	}
}

func WithBotToken(token string) ClientOption {
	return func(o *clientOptions) {
		o.botToken = token
	}
}

func applyClientOptions(opts []ClientOption) clientOptions {
	var out clientOptions

	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}

	out.Timeout = defaultPositiveDuration(out.Timeout, 10*time.Second)
	out.DialTimeout = defaultPositiveDuration(out.DialTimeout, 3*time.Second)
	out.TLSHandshakeTimeout = defaultPositiveDuration(out.TLSHandshakeTimeout, 5*time.Second)
	out.ResponseHeaderTimeout = defaultPositiveDuration(out.ResponseHeaderTimeout, 5*time.Second)
	out.IdleConnTimeout = defaultPositiveDuration(out.IdleConnTimeout, 90*time.Second)
	out.MaxIdleConns = defaultPositiveInt(out.MaxIdleConns, 10)
	out.MaxIdleConnsPerHost = defaultPositiveInt(out.MaxIdleConnsPerHost, 10)
	out.ReadIdleTimeout = defaultPositiveDuration(out.ReadIdleTimeout, 30*time.Second)
	out.PingTimeout = defaultPositiveDuration(out.PingTimeout, 15*time.Second)
	out.PingProbeTimeout = defaultPositiveDuration(out.PingProbeTimeout, 5*time.Second)

	out.WriteByteTimeout = defaultPositiveDuration(out.WriteByteTimeout, 10*time.Second)
	if out.ReplyRetryMax < 0 {
		out.ReplyRetryMax = 0
	}

	return out
}

func defaultPositiveDuration(v, fallback time.Duration) time.Duration {
	if v > 0 {
		return v
	}

	return fallback
}

func defaultPositiveInt(v, fallback int) int {
	if v > 0 {
		return v
	}

	return fallback
}
