package client

import (
	"log/slog"
	"time"
)

type clientOptions struct {
	Transport             string
	Timeout               time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	ReadIdleTimeout       time.Duration
	PingTimeout           time.Duration
	WriteByteTimeout      time.Duration
	Logger                *slog.Logger
	ReplyRetryMax         int // 0 = disabled (default), >0 = max attempts for 429 retry
}

type ClientOption func(*clientOptions)

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

func WithReplyRetry(maxAttempts int) ClientOption {
	return func(o *clientOptions) {
		o.ReplyRetryMax = maxAttempts
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
