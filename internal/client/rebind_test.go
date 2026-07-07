package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	target.Store(first.URL)
	c := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return target.Load().(string), nil },
		BotToken:       "bot-token",
		ClientOptions:  []ClientOption{WithHTTPClient(first.Client()), WithTransport("http1")},
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

	first, err := c.current()
	if err != nil {
		t.Fatalf("first current() error = %v", err)
	}
	second, err := c.current()
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

	if _, err := c.current(); err == nil {
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
	if _, err := c.current(); err != nil {
		t.Fatalf("current() before Close error = %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := c.current()
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

	_, err := c.current()
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("current() error = %v, want containing %q", err, wantErr)
	}
}

func TestRebindingClientCloseDoesNotWaitForResolver(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
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
		_, err := c.current()
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

	close(release)
	err := <-currentDone
	if err == nil || !strings.Contains(err.Error(), "client is closed") {
		t.Fatalf("current() error = %v, want closed error", err)
	}
}

type resolverTestError struct{ msg string }

func (e *resolverTestError) Error() string { return e.msg }
