package client

import (
	"log/slog"
	"testing"
	"time"
)

func TestApplySendOptions(t *testing.T) {
	threadID := "12345"
	threadScope := 2

	tests := []struct {
		name string
		opts []SendOption
		want sendOptions
	}{
		{
			name: "empty options",
			opts: nil,
			want: sendOptions{},
		},
		{
			name: "thread id only",
			opts: []SendOption{WithThreadID(threadID)},
			want: sendOptions{ThreadID: &threadID},
		},
		{
			name: "thread scope only",
			opts: []SendOption{WithThreadScope(threadScope)},
			want: sendOptions{ThreadScope: &threadScope},
		},
		{
			name: "both options",
			opts: []SendOption{WithThreadID(threadID), WithThreadScope(threadScope)},
			want: sendOptions{ThreadID: &threadID, ThreadScope: &threadScope},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSendOptionsEqual(t, applySendOptions(tt.opts), tt.want)
		})
	}
}

func TestValidateSendOptionsValidCases(t *testing.T) {
	threadID := "12345"
	one := 1
	two := 2

	tests := []validateSendOptionsSuccessCase{
		{
			name:  "empty options",
			input: sendOptions{},
		},
		{
			name:  "valid thread id only",
			input: sendOptions{ThreadID: &threadID},
		},
		{
			name:  "valid scope one without thread id",
			input: sendOptions{ThreadScope: &one},
		},
		{
			name:  "valid scope two with thread id",
			input: sendOptions{ThreadID: &threadID, ThreadScope: &two},
		},
	}

	runValidateSendOptionsSuccessTests(t, tests)
}

func TestValidateSendOptionsInvalidCases(t *testing.T) {
	threadID := "12a45"
	zero := 0
	negative := -1
	two := 2

	tests := []validateSendOptionsErrorCase{
		{
			name:    "reject non numeric thread id",
			input:   sendOptions{ThreadID: &threadID},
			wantErr: `iris: threadId must be numeric, got "12a45"`,
		},
		{
			name:    "reject zero thread scope",
			input:   sendOptions{ThreadScope: &zero},
			wantErr: "iris: threadScope must be positive, got 0",
		},
		{
			name:    "reject negative thread scope",
			input:   sendOptions{ThreadScope: &negative},
			wantErr: "iris: threadScope must be positive, got -1",
		},
		{
			name:    "reject scope two without thread id",
			input:   sendOptions{ThreadScope: &two},
			wantErr: "iris: threadScope >= 2 requires threadId",
		},
	}

	runValidateSendOptionsErrorTests(t, tests)
}

type validateSendOptionsSuccessCase struct {
	name  string
	input sendOptions
}

func runValidateSendOptionsSuccessTests(t *testing.T, tests []validateSendOptionsSuccessCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSendOptions(tt.input); err != nil {
				t.Fatalf("validateSendOptions() error = %v, want nil", err)
			}
		})
	}
}

type validateSendOptionsErrorCase struct {
	name    string
	input   sendOptions
	wantErr string
}

func runValidateSendOptionsErrorTests(t *testing.T, tests []validateSendOptionsErrorCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSendOptions(tt.input)
			if err == nil {
				t.Fatalf("validateSendOptions() error = nil, want %q", tt.wantErr)
			}

			if err.Error() != tt.wantErr {
				t.Fatalf("validateSendOptions() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func assertSendOptionsEqual(t *testing.T, got, want sendOptions) {
	t.Helper()

	if !equalStringPtr(got.ThreadID, want.ThreadID) {
		t.Fatalf("ThreadID = %v, want %v", got.ThreadID, want.ThreadID)
	}

	if !equalIntPtr(got.ThreadScope, want.ThreadScope) {
		t.Fatalf("ThreadScope = %v, want %v", got.ThreadScope, want.ThreadScope)
	}
}

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

func stringPtr(s string) *string {
	return &s
}

func equalStringPtr(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func equalIntPtr(got, want *int) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}
