package transport

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dialGuardContextKey struct{}

type dialGuardClock struct {
	nanos atomic.Int64
}

func newDialGuardClock(now time.Time) *dialGuardClock {
	clock := &dialGuardClock{}
	clock.nanos.Store(now.UnixNano())
	return clock
}

func (c *dialGuardClock) Now() time.Time {
	return time.Unix(0, c.nanos.Load())
}

func (c *dialGuardClock) Advance(d time.Duration) {
	c.nanos.Add(int64(d))
}

type dialGuardResolver struct {
	mu      sync.Mutex
	results []dialGuardResolveResult
	calls   int
}

type dialGuardResolveResult struct {
	ips      []net.IP
	err      error
	panicVal any
	started  chan<- context.Context
	release  <-chan struct{}
}

func (r *dialGuardResolver) LookupIP(ctx context.Context, _ string) ([]net.IP, error) {
	r.mu.Lock()
	call := r.calls
	r.calls++
	result := r.results[call]
	r.mu.Unlock()

	if result.started != nil {
		result.started <- ctx
	}
	if result.panicVal != nil {
		panic(result.panicVal)
	}
	if result.release != nil {
		select {
		case <-result.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return result.ips, result.err
}

func (r *dialGuardResolver) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type dialGuardLogHandler struct {
	mu      sync.Mutex
	records []dialGuardLogRecord
	notify  chan struct{}
}

type dialGuardLogRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

func (h *dialGuardLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *dialGuardLogHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, dialGuardLogRecord{level: record.Level, message: record.Message, attrs: attrs})
	h.mu.Unlock()
	select {
	case h.notify <- struct{}{}:
	default:
	}
	return nil
}

func (h *dialGuardLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *dialGuardLogHandler) WithGroup(string) slog.Handler      { return h }

func (h *dialGuardLogHandler) Last() dialGuardLogRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.records[len(h.records)-1]
}

func newTestH3DialGuard(
	t *testing.T,
	ctx context.Context,
	baseURL string,
	clock *dialGuardClock,
	resolver *dialGuardResolver,
	opts ...H3DialGuardOption,
) (func(context.Context, net.IP) error, error) {
	t.Helper()
	return newH3DialGuardForBaseURL(ctx, baseURL, h3DialGuardDependencies{
		lookupIP: resolver.LookupIP,
		now:      clock.Now,
	}, opts...)
}

func TestH3DialGuardLiteralIPFastPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		host    string
		allowed net.IP
		denied  net.IP
	}{
		{name: "IPv4", baseURL: "https://192.0.2.10:31001", host: "192.0.2.10", allowed: net.ParseIP("192.0.2.10"), denied: net.ParseIP("192.0.2.11")},
		{name: "IPv6", baseURL: "https://[2001:db8::10]:31001", host: "2001:db8::10", allowed: net.ParseIP("2001:db8::10"), denied: net.ParseIP("2001:db8::11")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newDialGuardClock(time.Unix(1, 0))
			resolver := &dialGuardResolver{}
			guard, err := newTestH3DialGuard(t, t.Context(), tt.baseURL, clock, resolver)
			if err != nil {
				t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
			}
			if err := guard(t.Context(), tt.allowed); err != nil {
				t.Fatalf("guard(allowed) error = %v", err)
			}
			if err := guard(t.Context(), tt.denied); err == nil || !strings.Contains(err.Error(), tt.host) {
				t.Fatalf("guard(denied) error = %v, want host-bearing denial", err)
			}
			if resolver.Calls() != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolver.Calls())
			}
		})
	}
}

func TestH3DialGuardRefreshReplacesAllowset(t *testing.T) {
	t.Parallel()

	oldIP := net.ParseIP("192.0.2.20")
	newIP := net.ParseIP("192.0.2.21")
	refreshDone := make(chan context.Context, 1)
	clock := newDialGuardClock(time.Unix(2, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{ips: []net.IP{oldIP}},
		{ips: []net.IP{newIP}, started: refreshDone},
	}}
	guard, err := newTestH3DialGuard(t, t.Context(), "https://iris.test:31001", clock, resolver)
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP) error = %v", err)
	}
	<-refreshDone
	waitForDialGuard(t, func() bool { return guard(t.Context(), newIP) == nil })
	if err := guard(t.Context(), oldIP); err == nil {
		t.Fatal("guard(old IP) error = nil after allowset replacement")
	}
}

func TestH3DialGuardConcurrentRefreshUsesStaleAllowsetAndSingleFlight(t *testing.T) {
	oldIP := net.ParseIP("192.0.2.30")
	newIP := net.ParseIP("192.0.2.31")
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	clock := newDialGuardClock(time.Unix(3, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{ips: []net.IP{oldIP}},
		{ips: []net.IP{newIP}, started: started, release: release},
	}}
	guard, err := newTestH3DialGuard(t, t.Context(), "https://iris.test:31001", clock, resolver, WithH3DialGuardTTL(time.Minute))
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("refresh leader stale decision error = %v", err)
	}
	<-started

	const dialCount = 16
	errs := make(chan error, dialCount)
	for range dialCount {
		go func() { errs <- guard(t.Context(), oldIP) }()
	}
	for range dialCount {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("concurrent stale decision error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent stale decision blocked on DNS refresh")
		}
	}
	if resolver.Calls() != 2 {
		t.Fatalf("resolver calls = %d, want 2", resolver.Calls())
	}

	close(release)
	waitForDialGuard(t, func() bool { return guard(t.Context(), newIP) == nil })
}

func TestH3DialGuardRefreshPanicAllowsLaterRecovery(t *testing.T) {
	oldIP := net.ParseIP("192.0.2.35")
	newIP := net.ParseIP("192.0.2.36")
	panicStarted := make(chan context.Context, 1)
	recoveryStarted := make(chan context.Context, 1)
	handler := &dialGuardLogHandler{notify: make(chan struct{}, 1)}
	clock := newDialGuardClock(time.Unix(35, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{ips: []net.IP{oldIP}},
		{panicVal: "resolver panic", started: panicStarted},
		{ips: []net.IP{newIP}, started: recoveryStarted},
	}}
	guard, err := newTestH3DialGuard(
		t,
		t.Context(),
		"https://iris.test:31001",
		clock,
		resolver,
		WithH3DialGuardTTL(time.Minute),
		WithH3DialGuardLogger(slog.New(handler)),
	)
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP) error = %v", err)
	}
	<-panicStarted
	select {
	case <-handler.notify:
	case <-time.After(time.Second):
		t.Fatal("refresh panic was not logged")
	}
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP before next TTL) error = %v", err)
	}
	select {
	case <-recoveryStarted:
		t.Fatal("refresh retried before next TTL after resolver panic")
	case <-time.After(20 * time.Millisecond):
	}
	if resolver.Calls() != 2 {
		t.Fatalf("resolver calls = %d, want 2 before next TTL", resolver.Calls())
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP after panic) error = %v", err)
	}
	select {
	case <-recoveryStarted:
	case <-time.After(time.Second):
		t.Fatal("refresh did not recover after resolver panic")
	}
	waitForDialGuard(t, func() bool { return guard(t.Context(), newIP) == nil })
}

func TestH3DialGuardRefreshFailureKeepsStaleAllowsetAndWarns(t *testing.T) {
	t.Parallel()

	oldIP := net.ParseIP("192.0.2.40")
	unknownIP := net.ParseIP("192.0.2.41")
	resolveErr := errors.New("temporary DNS failure")
	started := make(chan context.Context, 1)
	clock := newDialGuardClock(time.Unix(4, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{ips: []net.IP{oldIP}},
		{err: resolveErr, started: started},
	}}
	handler := &dialGuardLogHandler{notify: make(chan struct{}, 1)}
	guard, err := newTestH3DialGuard(
		t,
		t.Context(),
		"https://iris.test:31001",
		clock,
		resolver,
		WithH3DialGuardTTL(time.Minute),
		WithH3DialGuardLogger(slog.New(handler)),
	)
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP) error = %v", err)
	}
	<-started
	select {
	case <-handler.notify:
	case <-time.After(time.Second):
		t.Fatal("refresh failure warning was not logged")
	}
	if err := guard(t.Context(), oldIP); err != nil {
		t.Fatalf("guard(stale IP after failure) error = %v", err)
	}
	if err := guard(t.Context(), unknownIP); err == nil {
		t.Fatal("guard(unknown IP) error = nil after refresh failure")
	}
	if resolver.Calls() != 2 {
		t.Fatalf("resolver calls = %d, want 2 before next TTL", resolver.Calls())
	}
	record := handler.Last()
	if record.level != slog.LevelWarn || record.attrs["host"] != "iris.test" || !errors.Is(record.attrs["err"].(error), resolveErr) {
		t.Fatalf("warning = %+v, want WARN with host and err", record)
	}
}

func TestH3DialGuardRefreshDetachesCancellationAndPreservesValues(t *testing.T) {
	t.Parallel()

	oldIP := net.ParseIP("192.0.2.50")
	newIP := net.ParseIP("192.0.2.51")
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	clock := newDialGuardClock(time.Unix(5, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{ips: []net.IP{oldIP}},
		{ips: []net.IP{newIP}, started: started, release: release},
	}}
	guard, err := newTestH3DialGuard(t, t.Context(), "https://iris.test:31001", clock, resolver, WithH3DialGuardTTL(time.Minute))
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}

	clock.Advance(time.Minute)
	dialCtx, cancel := context.WithCancel(context.WithValue(t.Context(), dialGuardContextKey{}, "dial-value"))
	cancel()
	if err := guard(dialCtx, oldIP); err != nil {
		t.Fatalf("guard(stale IP) error = %v", err)
	}
	refreshCtx := <-started
	if got := refreshCtx.Value(dialGuardContextKey{}); got != "dial-value" {
		t.Fatalf("refresh context value = %v, want dial-value", got)
	}
	if err := refreshCtx.Err(); err != nil {
		t.Fatalf("refresh context error = %v, want nil", err)
	}
	close(release)
	waitForDialGuard(t, func() bool { return guard(t.Context(), newIP) == nil })
}

func TestH3DialGuardRejectsInvalidBaseURLAndNilIP(t *testing.T) {
	t.Parallel()

	clock := newDialGuardClock(time.Unix(6, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{{ips: []net.IP{net.ParseIP("192.0.2.60")}}}}
	for _, baseURL := range []string{"://bad", "/relative/path", "https:///missing-host"} {
		if _, err := newTestH3DialGuard(t, t.Context(), baseURL, clock, resolver); err == nil {
			t.Fatalf("newH3DialGuardForBaseURL(%q) error = nil", baseURL)
		}
	}

	guard, err := newTestH3DialGuard(t, t.Context(), "https://iris.test:31001", clock, resolver)
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}
	if err := guard(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "iris.test") {
		t.Fatalf("guard(nil) error = %v, want host-bearing denial", err)
	}
}

func TestH3DialGuardResolveTimeout(t *testing.T) {
	t.Parallel()

	clock := newDialGuardClock(time.Unix(7, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{{release: make(chan struct{})}}}
	_, err := newTestH3DialGuard(
		t,
		t.Context(),
		"https://iris.test:31001",
		clock,
		resolver,
		WithH3DialGuardResolveTimeout(time.Millisecond),
	)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v, want context deadline exceeded", err)
	}
}

func TestH3DialGuardFailFastInitialResolve(t *testing.T) {
	t.Parallel()

	resolveErr := errors.New("initial DNS failure")
	clock := newDialGuardClock(time.Unix(7, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{{err: resolveErr}}}
	_, err := newTestH3DialGuard(t, t.Context(), "https://iris.test:31001", clock, resolver)
	if err == nil || !errors.Is(err, resolveErr) || !strings.Contains(err.Error(), "iris.test") {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v, want host-bearing resolve error", err)
	}
}

func TestH3DialGuardLenientInitDeniesUntilTTLRecovery(t *testing.T) {
	t.Parallel()

	allowedIP := net.ParseIP("192.0.2.70")
	resolveErr := errors.New("initial DNS failure")
	started := make(chan context.Context, 1)
	clock := newDialGuardClock(time.Unix(8, 0))
	resolver := &dialGuardResolver{results: []dialGuardResolveResult{
		{err: resolveErr},
		{ips: []net.IP{allowedIP}, started: started},
	}}
	handler := &dialGuardLogHandler{notify: make(chan struct{}, 1)}
	guard, err := newTestH3DialGuard(
		t,
		t.Context(),
		"https://iris.test:31001",
		clock,
		resolver,
		WithH3DialGuardTTL(time.Minute),
		WithH3DialGuardLenientInit(),
		WithH3DialGuardLogger(slog.New(handler)),
	)
	if err != nil {
		t.Fatalf("newH3DialGuardForBaseURL() error = %v", err)
	}
	if err := guard(t.Context(), allowedIP); err == nil {
		t.Fatal("guard(allowed IP) error = nil before recovery")
	}
	if resolver.Calls() != 1 {
		t.Fatalf("resolver calls = %d, want 1 before TTL", resolver.Calls())
	}

	clock.Advance(time.Minute)
	if err := guard(t.Context(), allowedIP); err == nil {
		t.Fatal("guard(allowed IP) error = nil while refresh uses deny-all stale allowset")
	}
	<-started
	waitForDialGuard(t, func() bool { return guard(t.Context(), allowedIP) == nil })
	if record := handler.Last(); record.level != slog.LevelWarn || record.attrs["host"] != "iris.test" || !errors.Is(record.attrs["err"].(error), resolveErr) {
		t.Fatalf("initial warning = %+v, want WARN with host and err", record)
	}
}

func TestWithH3DialGuardForBaseURLWiresContextGuard(t *testing.T) {
	t.Parallel()

	opt, err := withH3DialGuardForBaseURL(t.Context(), "https://192.0.2.80:31001")
	if err != nil {
		t.Fatalf("WithH3DialGuardForBaseURL() error = %v", err)
	}
	got := applyClientOptions([]ClientOption{opt})
	if got.h3DialGuardContext == nil || got.h3DialGuard != nil {
		t.Fatalf("dial guard options = context:%v plain:%v, want only context guard", got.h3DialGuardContext != nil, got.h3DialGuard != nil)
	}
}

func waitForDialGuard(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for dial guard state")
}
