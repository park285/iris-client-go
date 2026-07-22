package rebind

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRebindingClientSwapsOnBaseURLChange(t *testing.T) {
	t.Parallel()

	var firstCalls, secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathReply {
			t.Errorf("first server path = %q, want %q", r.URL.Path, PathReply)
		}
		firstCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathReply {
			t.Errorf("second server path = %q, want %q", r.URL.Path, PathReply)
		}
		secondCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer second.Close()

	var target atomic.Value
	var resolveCalls atomic.Int32
	target.Store(first.URL)
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			return target.Load().(string), nil
		},
		BotToken:      "bot-token",
		ClientOptions: []ClientOption{WithHTTPClient(first.Client()), WithTransport("http1")},
	})
	defer func() { _ = c.Close() }()

	if err := c.SendMessage(context.Background(), "room-1", "hello"); err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	if got := firstCalls.Load(); got != 1 {
		t.Fatalf("first calls = %d, want 1", got)
	}

	target.Store(second.URL)
	if err := c.SendMessage(context.Background(), "room-1", "world"); err != nil {
		t.Fatalf("second SendMessage() error = %v", err)
	}
	if got := secondCalls.Load(); got != 1 {
		t.Fatalf("second calls = %d, want 1", got)
	}
	if got := resolveCalls.Load(); got != 2 {
		t.Fatalf("resolve calls with zero interval = %d, want 2", got)
	}
}

func TestRebindingClientCachesResolutionUntilIntervalExpires(t *testing.T) {
	t.Parallel()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	const interval = time.Minute
	var target atomic.Value
	var resolveCalls atomic.Int32
	target.Store(first.URL)
	now := time.Unix(1_700_000_000, 0)
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			return target.Load().(string), nil
		},
		ResolveInterval: interval,
		BotToken:        "bot-token",
		ClientOptions:   []ClientOption{WithHTTPClient(first.Client()), WithTransport("http1")},
	})
	c.now = func() time.Time { return now }
	defer func() { _ = c.Close() }()

	firstClient, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("first current() error = %v", err)
	}
	target.Store(second.URL)
	cachedClient, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("cached current() error = %v", err)
	}
	if cachedClient != firstClient {
		t.Fatal("current() replaced the client before ResolveInterval expired")
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls before expiry = %d, want 1", got)
	}

	now = now.Add(interval)
	refreshedClient, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("current() after expiry error = %v", err)
	}
	if refreshedClient == firstClient {
		t.Fatal("current() kept the stale client after ResolveInterval expired")
	}
	if got := resolveCalls.Load(); got != 2 {
		t.Fatalf("resolve calls after expiry = %d, want 2", got)
	}
}

func TestRebindingClientCachesResolverErrorUntilIntervalExpires(t *testing.T) {
	t.Parallel()

	const interval = time.Minute
	var resolveCalls atomic.Int32
	now := time.Unix(1_700_000_000, 0)
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			return "", &resolverTestError{msg: "resolver boom"}
		},
		ResolveInterval: interval,
		BotToken:        "bot-token",
	})
	c.now = func() time.Time { return now }
	defer func() { _ = c.Close() }()

	for call := 1; call <= 2; call++ {
		_, err := c.current(context.Background())
		if err == nil || !strings.Contains(err.Error(), "resolver boom") {
			t.Fatalf("current() call %d error = %v, want resolver error", call, err)
		}
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls before error snapshot expiry = %d, want 1", got)
	}

	now = now.Add(interval)
	_, err := c.current(context.Background())
	if err == nil || !strings.Contains(err.Error(), "resolver boom") {
		t.Fatalf("current() after error snapshot expiry = %v, want resolver error", err)
	}
	if got := resolveCalls.Load(); got != 2 {
		t.Fatalf("resolve calls after error snapshot expiry = %d, want 2", got)
	}
}

func TestRebindingClientCoalescesConcurrentRefresh(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	const callers = 32
	start := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseResolver := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseResolver()
	var ready sync.WaitGroup
	ready.Add(callers)
	var resolveCalls atomic.Int32
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			<-release
			return server.URL, nil
		},
		BotToken:      "bot-token",
		ClientOptions: []ClientOption{WithHTTPClient(server.Client()), WithTransport("http1")},
	})
	defer func() { _ = c.Close() }()

	type result struct {
		client *H2CClient
		err    error
	}
	results := make(chan result, callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			client, err := c.current(context.Background())
			results <- result{client: client, err: err}
		}()
	}
	ready.Wait()
	close(start)

	deadline := time.Now().Add(time.Second)
	for resolveCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(25 * time.Millisecond)
	if got := resolveCalls.Load(); got != 1 {
		releaseResolver()
		t.Fatalf("concurrent resolve calls = %d, want 1", got)
	}
	releaseResolver()

	var firstClient *H2CClient
	for range callers {
		result := <-results
		if result.err != nil {
			t.Fatalf("current() error = %v", result.err)
		}
		if firstClient == nil {
			firstClient = result.client
		} else if result.client != firstClient {
			t.Fatalf("concurrent current() returned different clients: %p vs %p", firstClient, result.client)
		}
	}
}

func TestRebindingClientCoalescedFollowerHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseResolver := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseResolver()
	var resolveCalls atomic.Int32
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			close(started)
			<-release
			return server.URL, nil
		},
		BotToken:      "bot-token",
		ClientOptions: []ClientOption{WithHTTPClient(server.Client()), WithTransport("http1")},
	})
	defer func() { _ = c.Close() }()

	leaderDone := make(chan error, 1)
	go func() {
		_, err := c.current(context.Background())
		leaderDone <- err
	}()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := c.GetConfig(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetConfig() error = %v, want context deadline exceeded", err)
	}
	select {
	case err := <-leaderDone:
		t.Fatalf("leader completed before resolver release with error %v", err)
	default:
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls = %d, want 1", got)
	}

	releaseResolver()
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader current() error = %v", err)
	}
}

func TestRebindingClientRefreshLeaderHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseResolver := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseResolver()
	var resolveCalls atomic.Int32
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			close(started)
			<-release
			return server.URL, nil
		},
		ResolveInterval: time.Minute,
		BotToken:        "bot-token",
		ClientOptions:   []ClientOption{WithHTTPClient(server.Client()), WithTransport("http1")},
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	type result struct {
		client *H2CClient
		err    error
	}
	leaderDone := make(chan result, 1)
	go func() {
		client, err := c.current(ctx)
		leaderDone <- result{client: client, err: err}
	}()
	<-started

	select {
	case got := <-leaderDone:
		if !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("leader current() error = %v, want context deadline exceeded", got.err)
		}
		if got.client != nil {
			t.Fatalf("leader current() client = %p, want nil", got.client)
		}
	case <-time.After(500 * time.Millisecond):
		releaseResolver()
		got := <-leaderDone
		t.Fatalf("leader current() did not honor its deadline before resolver release; result = %#v", got)
	}

	followerDone := make(chan result, 1)
	go func() {
		client, err := c.current(context.Background())
		followerDone <- result{client: client, err: err}
	}()
	select {
	case got := <-followerDone:
		t.Fatalf("follower current() completed before resolver release; result = %#v", got)
	case <-time.After(10 * time.Millisecond):
	}

	releaseResolver()
	follower := <-followerDone
	if follower.err != nil {
		t.Fatalf("follower current() error = %v", follower.err)
	}
	if follower.client == nil {
		t.Fatal("follower current() client = nil")
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls after coalesced refresh = %d, want 1", got)
	}

	cached, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("cached current() error = %v", err)
	}
	if cached != follower.client {
		t.Fatalf("cached current() client = %p, want follower client %p", cached, follower.client)
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls inside ResolveInterval = %d, want 1", got)
	}
}

func TestRebindingClientCanceledLeaderDoesNotStartRefresh(t *testing.T) {
	t.Parallel()

	var resolveCalls atomic.Int32
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			return "https://iris.example", nil
		},
		BotToken: "bot-token",
	})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.current(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("current() error = %v, want context canceled", err)
	}

	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	if refresh != nil {
		t.Fatal("canceled leader left a refresh in flight")
	}
	if got := resolveCalls.Load(); got != 0 {
		t.Fatalf("resolve calls for canceled leader = %d, want 0", got)
	}
}

func TestRebindingClientRefreshPanicCompletesErrorSnapshot(t *testing.T) {
	t.Parallel()

	var resolveCalls atomic.Int32
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			resolveCalls.Add(1)
			panic("resolver boom")
		},
		ResolveInterval: time.Minute,
		BotToken:        "bot-token",
	})
	defer func() { _ = c.Close() }()

	for call := 1; call <= 2; call++ {
		_, err := c.current(context.Background())
		if err == nil || !strings.Contains(err.Error(), "refresh panicked") {
			t.Fatalf("current() call %d error = %v, want refresh panic error", call, err)
		}
	}
	if got := resolveCalls.Load(); got != 1 {
		t.Fatalf("resolve calls for cached panic snapshot = %d, want 1", got)
	}
}

func TestRebindingClientReusesClientForSameBaseURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return server.URL, nil },
		BotToken:       "bot-token",
		ClientOptions:  []ClientOption{WithHTTPClient(server.Client())},
	})
	defer func() { _ = c.Close() }()

	first, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("first current() error = %v", err)
	}
	second, err := c.current(context.Background())
	if err != nil {
		t.Fatalf("second current() error = %v", err)
	}
	if first != second {
		t.Fatalf("current() returned different clients for same base URL: %p vs %p", first, second)
	}
}

func TestRebindingClientDoesNotPoisonCacheOnInitFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	previous := NewH2CClient(server.URL, "bot-token", WithHTTPClient(server.Client()))
	if err := previous.InitError(); err != nil {
		t.Fatalf("seed client init error = %v", err)
	}

	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return "https://iris.example", nil },
		BotToken:       "bot-token",
		ClientOptions: []ClientOption{
			WithTransport("h3"),
			WithH3CACertFile(filepath.Join(t.TempDir(), "missing-ca.pem")),
		},
	})
	c.cachedURL = server.URL
	c.cached = previous

	if _, err := c.current(context.Background()); err == nil {
		t.Fatal("current() error = nil, want H3 CA initialization error")
	}
	if c.cached != previous || c.cachedURL != server.URL {
		t.Fatal("failed reload poisoned the previously cached client")
	}
}

func TestRebindingClientCloseFlushesPendingStaleClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL:  func() (string, error) { return server.URL, nil },
		BotToken:        "bot-token",
		StaleCloseGrace: time.Hour,
		ClientOptions:   []ClientOption{WithHTTPClient(server.Client())},
	})

	stale := NewH2CClient(server.URL, "bot-token", WithHTTPClient(server.Client()))
	if err := stale.InitError(); err != nil {
		t.Fatalf("stale client init error = %v", err)
	}

	c.mu.Lock()
	c.scheduleStaleCloseLocked(stale)
	c.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- c.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not flush pending stale close within 5s (closeSignal not honored)")
	}
}

func TestRebindingClientStaleClosePanicDoesNotBlockClose(t *testing.T) {
	t.Parallel()

	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return "https://iris.example", nil },
		BotToken:       "bot-token",
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	stale := panicTestCloser{}

	c.staleClosers.Add(1)
	go c.runStaleClose(stale, 0)

	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() remained blocked after stale client close panic")
	}
}

func TestRebindingClientDoesNotScheduleNilStaleClient(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return "https://iris.example", nil },
		BotToken:       "bot-token",
		Logger:         slog.New(slog.NewTextHandler(&logs, nil)),
	})

	c.mu.Lock()
	c.scheduleStaleCloseLocked(nil)
	c.mu.Unlock()

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if logs.Len() != 0 {
		t.Fatalf("nil stale client emitted logs: %s", logs.String())
	}
}

func TestRebindingClientClosedReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return server.URL, nil },
		BotToken:       "bot-token",
		ClientOptions:  []ClientOption{WithHTTPClient(server.Client())},
	})
	if _, err := c.current(context.Background()); err != nil {
		t.Fatalf("current() before Close error = %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := c.current(context.Background())
	if err == nil || !strings.Contains(err.Error(), "client is closed") {
		t.Fatalf("current() after Close error = %v, want containing %q", err, "client is closed")
	}
}

func TestRebindingClientResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := "resolver boom"
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return "", &resolverTestError{msg: wantErr} },
		BotToken:       "bot-token",
	})
	defer func() { _ = c.Close() }()

	_, err := c.current(context.Background())
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("current() error = %v, want containing %q", err, wantErr)
	}
}

func TestRebindingClientCloseDoesNotWaitForResolver(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseResolver := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseResolver()
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) {
			close(started)
			<-release
			return "https://iris.example", nil
		},
		BotToken: "bot-token",
	})

	currentDone := make(chan error, 1)
	go func() {
		_, err := c.current(context.Background())
		currentDone <- err
	}()

	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- c.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() waited for ResolveBaseURL")
	}

	select {
	case err := <-currentDone:
		if err == nil || !strings.Contains(err.Error(), "client is closed") {
			t.Fatalf("current() error = %v, want closed error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("current() did not observe Close while resolver remained blocked")
	}

	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	if refresh == nil {
		t.Fatal("blocked resolver refresh disappeared before callback returned")
	}

	releaseResolver()
	select {
	case <-refresh.done:
	case <-time.After(time.Second):
		t.Fatal("refresh did not complete after resolver returned")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refresh != nil {
		t.Fatal("refresh remained in flight after resolver returned")
	}
	if !c.resolveValidUntil.IsZero() || c.resolveErr != nil {
		t.Fatalf("Close() snapshot restored after refresh completion: validUntil=%v err=%v", c.resolveValidUntil, c.resolveErr)
	}
}

type resolverTestError struct{ msg string }

func (e *resolverTestError) Error() string { return e.msg }

type panicTestCloser struct{}

func (panicTestCloser) Close() error { panic("close boom") }
