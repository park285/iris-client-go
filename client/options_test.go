package client

import (
	"log/slog"
	"testing"
	"time"
)

func TestApplyClientOptionsDefaults(t *testing.T) {
	got := applyClientOptions(nil)
	assertClientOptionsCore(t, got, clientOptions{
		Timeout:               10 * time.Second,
		DialTimeout:           3 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   10,
		ReadIdleTimeout:       30 * time.Second,
		PingTimeout:           15 * time.Second,
		WriteByteTimeout:      10 * time.Second,
	})

	if got.Transport != "" {
		t.Fatalf("Transport = %q, want empty", got.Transport)
	}

	if got.Logger != nil {
		t.Fatalf("Logger = %v, want nil", got.Logger)
	}

	if got.ReplyRetryMax != 0 {
		t.Fatalf("ReplyRetryMax = %d, want 0", got.ReplyRetryMax)
	}
}

func TestApplyClientOptionsOverrides(t *testing.T) {
	logger := slog.Default()
	got := applyClientOptions([]ClientOption{
		WithTransport("http1"),
		WithTimeout(2 * time.Second),
		WithDialTimeout(4 * time.Second),
		WithTLSHandshakeTimeout(6 * time.Second),
		WithResponseHeaderTimeout(7 * time.Second),
		WithIdleConnTimeout(8 * time.Second),
		WithMaxIdleConns(11),
		WithMaxIdleConnsPerHost(12),
		WithReadIdleTimeout(13 * time.Second),
		WithPingTimeout(14 * time.Second),
		WithWriteByteTimeout(15 * time.Second),
		WithLogger(logger),
		WithReplyRetry(3),
	})

	assertClientOptionsCore(t, got, clientOptions{
		Transport:             "http1",
		Timeout:               2 * time.Second,
		DialTimeout:           4 * time.Second,
		TLSHandshakeTimeout:   6 * time.Second,
		ResponseHeaderTimeout: 7 * time.Second,
		IdleConnTimeout:       8 * time.Second,
		MaxIdleConns:          11,
		MaxIdleConnsPerHost:   12,
		ReadIdleTimeout:       13 * time.Second,
		PingTimeout:           14 * time.Second,
		WriteByteTimeout:      15 * time.Second,
	})

	if got.Logger != logger {
		t.Fatalf("Logger = %v, want %v", got.Logger, logger)
	}

	if got.ReplyRetryMax != 3 {
		t.Fatalf("ReplyRetryMax = %d, want 3", got.ReplyRetryMax)
	}
}

func TestApplyClientOptionsFallbackForNonPositiveValues(t *testing.T) {
	got := applyClientOptions([]ClientOption{
		WithTimeout(0),
		WithDialTimeout(-1),
		WithTLSHandshakeTimeout(0),
		WithResponseHeaderTimeout(-1),
		WithIdleConnTimeout(0),
		WithMaxIdleConns(0),
		WithMaxIdleConnsPerHost(-1),
		WithReadIdleTimeout(0),
		WithPingTimeout(-1),
		WithWriteByteTimeout(0),
		WithReplyRetry(-1),
	})

	assertClientOptionsCore(t, got, clientOptions{
		Timeout:               10 * time.Second,
		DialTimeout:           3 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   10,
		ReadIdleTimeout:       30 * time.Second,
		PingTimeout:           15 * time.Second,
		WriteByteTimeout:      10 * time.Second,
	})

	if got.ReplyRetryMax != 0 {
		t.Fatalf("ReplyRetryMax = %d, want 0", got.ReplyRetryMax)
	}
}

func assertClientOptionsCore(t *testing.T, got, want clientOptions) {
	t.Helper()

	if got.Transport != want.Transport {
		t.Fatalf("Transport = %q, want %q", got.Transport, want.Transport)
	}

	if got.Timeout != want.Timeout {
		t.Fatalf("Timeout = %v, want %v", got.Timeout, want.Timeout)
	}

	if got.DialTimeout != want.DialTimeout {
		t.Fatalf("DialTimeout = %v, want %v", got.DialTimeout, want.DialTimeout)
	}

	if got.TLSHandshakeTimeout != want.TLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", got.TLSHandshakeTimeout, want.TLSHandshakeTimeout)
	}

	if got.ResponseHeaderTimeout != want.ResponseHeaderTimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", got.ResponseHeaderTimeout, want.ResponseHeaderTimeout)
	}

	if got.IdleConnTimeout != want.IdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", got.IdleConnTimeout, want.IdleConnTimeout)
	}

	if got.MaxIdleConns != want.MaxIdleConns {
		t.Fatalf("MaxIdleConns = %d, want %d", got.MaxIdleConns, want.MaxIdleConns)
	}

	if got.MaxIdleConnsPerHost != want.MaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", got.MaxIdleConnsPerHost, want.MaxIdleConnsPerHost)
	}

	if got.ReadIdleTimeout != want.ReadIdleTimeout {
		t.Fatalf("ReadIdleTimeout = %v, want %v", got.ReadIdleTimeout, want.ReadIdleTimeout)
	}

	if got.PingTimeout != want.PingTimeout {
		t.Fatalf("PingTimeout = %v, want %v", got.PingTimeout, want.PingTimeout)
	}

	if got.WriteByteTimeout != want.WriteByteTimeout {
		t.Fatalf("WriteByteTimeout = %v, want %v", got.WriteByteTimeout, want.WriteByteTimeout)
	}
}

func TestDefaultPositiveHelpers(t *testing.T) {
	if got := defaultPositiveDuration(2*time.Second, 5*time.Second); got != 2*time.Second {
		t.Fatalf("defaultPositiveDuration(positive) = %v, want 2s", got)
	}

	if got := defaultPositiveDuration(0, 5*time.Second); got != 5*time.Second {
		t.Fatalf("defaultPositiveDuration(zero) = %v, want 5s", got)
	}

	if got := defaultPositiveInt(3, 7); got != 3 {
		t.Fatalf("defaultPositiveInt(positive) = %d, want 3", got)
	}

	if got := defaultPositiveInt(-1, 7); got != 7 {
		t.Fatalf("defaultPositiveInt(negative) = %d, want 7", got)
	}
}
