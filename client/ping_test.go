package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsReplyReachableStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusMethodNotAllowed, want: true},
		{status: http.StatusUnauthorized, want: true},
		{status: http.StatusForbidden, want: true},
		{status: http.StatusBadRequest, want: true},
		{status: http.StatusOK, want: false},
		{status: http.StatusTooManyRequests, want: false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("status_%d", tt.status)
		t.Run(name, func(t *testing.T) {
			if got := isReplyReachableStatus(tt.status); got != tt.want {
				t.Fatalf("isReplyReachableStatus(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestPingReadySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}
}

func TestPingFallsBackToHealth(t *testing.T) {
	var readyCalls, healthCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathReady:
			readyCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case PathHealth:
			healthCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}

	if readyCalls.Load() != 1 || healthCalls.Load() != 1 {
		t.Fatalf("calls = ready:%d health:%d, want 1 each", readyCalls.Load(), healthCalls.Load())
	}
}

func TestPingFallsBackToReplyProbe(t *testing.T) {
	var readyCalls, healthCalls, replyCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathReady:
			readyCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case PathHealth:
			healthCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case PathReply:
			replyCalls.Add(1)

			if r.Method != http.MethodOptions {
				t.Fatalf("method = %s, want OPTIONS", r.Method)
			}

			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}

	if readyCalls.Load() != 1 || healthCalls.Load() != 1 || replyCalls.Load() != 1 {
		t.Fatalf("calls = ready:%d health:%d reply:%d, want 1 each", readyCalls.Load(), healthCalls.Load(), replyCalls.Load())
	}
}

func TestPingPermanentErrorStopsRetry(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	if client.Ping(t.Context()) {
		t.Fatal("Ping() = true, want false")
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestRetryPingRetriesTransientErrors(t *testing.T) {
	var attempts atomic.Int32

	ok := retryPing(t.Context(), nil, "http://example.com", func(context.Context) (bool, error) {
		current := attempts.Add(1)
		if current < 3 {
			return false, errors.New("temporary failure")
		}

		return true, nil
	})
	if !ok {
		t.Fatal("retryPing() = false, want true")
	}

	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func TestRetryPingStopsOnPermanentError(t *testing.T) {
	var attempts atomic.Int32

	ok := retryPing(t.Context(), nil, "http://example.com", func(context.Context) (bool, error) {
		attempts.Add(1)
		return false, &permanentPingError{err: errors.New("bad request")}
	})
	if ok {
		t.Fatal("retryPing() = true, want false")
	}

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestRetryPingHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var attempts atomic.Int32

	ok := retryPing(ctx, nil, "http://example.com", func(context.Context) (bool, error) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			cancel()
		}

		return false, errors.New("temporary failure")
	})
	if ok {
		t.Fatal("retryPing() = true, want false")
	}

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestRetryPingReturnsFalseWhenAllAttemptsFail(t *testing.T) {
	start := time.Now()

	var attempts atomic.Int32

	ok := retryPing(t.Context(), nil, "http://example.com", func(context.Context) (bool, error) {
		attempts.Add(1)
		return false, errors.New("temporary failure")
	})
	if ok {
		t.Fatal("retryPing() = true, want false")
	}

	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}

	if elapsed := time.Since(start); elapsed < 140*time.Millisecond {
		t.Fatalf("elapsed = %v, want at least about 150ms of backoff", elapsed)
	}
}
