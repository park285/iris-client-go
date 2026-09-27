package transport

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPingReadySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "", WithTransport(transportHTTP1))
	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}
}

// probe가 endpoint 하나뿐이므로 404는 "다음 endpoint로 넘어감" 신호가 아니라
// 잘못된 baseURL·경로를 뜻하는 영구 실패다. 조용히 false를 반환하지 않고 기록되어야 한다.
func TestPingReady404IsLoggedPermanentFailure(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var logs bytes.Buffer

	client := NewAPIClient(server.URL, "",
		WithTransport(transportHTTP1),
		WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)
	if client.Ping(t.Context()) {
		t.Fatal("Ping() = true, want false")
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (permanent failure must not retry)", calls.Load())
	}

	if !strings.Contains(logs.String(), "iris_ping_permanent_failure") {
		t.Fatalf("logs = %q, want iris_ping_permanent_failure for /ready 404", logs.String())
	}
}

func TestPingReady404DoesNotProbeOtherEndpoints(t *testing.T) {
	var readyCalls, healthCalls, replyCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathReady:
			readyCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case PathHealth:
			healthCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		case PathReply:
			replyCalls.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "", WithTransport(transportHTTP1))
	if client.Ping(t.Context()) {
		t.Fatal("Ping() = true, want false")
	}

	if readyCalls.Load() != 1 || healthCalls.Load() != 0 || replyCalls.Load() != 0 {
		t.Fatalf("calls = ready:%d health:%d reply:%d, want ready:1 health:0 reply:0", readyCalls.Load(), healthCalls.Load(), replyCalls.Load())
	}
}

func TestPingRespectsProbeTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "",
		WithTransport(transportHTTP1),
		WithPingProbeTimeout(50*time.Millisecond),
	)

	if client.Ping(t.Context()) {
		t.Fatal("Ping() = true, want false (probe timeout)")
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

	client := NewAPIClient(server.URL, "", WithTransport(transportHTTP1))
	if client.Ping(t.Context()) {
		t.Fatal("Ping() = true, want false")
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestPing_TransportFailure_WrapsAsTransportError(t *testing.T) {
	transportErr := errors.New("connection refused")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})
	client := NewAPIClient("http://localhost", "", WithRoundTripper(rt))

	err := client.probe(t.Context())
	if err == nil {
		t.Fatal("probe() error = nil, want transport error")
	}

	got, ok := errors.AsType[*TransportError](err)
	if !ok {
		t.Fatalf("probe() error does not wrap *TransportError: %v", err)
	}

	if got.Op != "ping" {
		t.Fatalf("TransportError.Op = %q, want ping", got.Op)
	}

	if got.URL != "http://localhost"+PathReady {
		t.Fatalf("TransportError.URL = %q, want http://localhost%s", got.URL, PathReady)
	}

	if !errors.Is(err, ErrTransport) {
		t.Fatal("probe() error must match ErrTransport")
	}

	if !errors.Is(err, ErrRetryable) {
		t.Fatal("probe() transport error must match ErrRetryable")
	}
}

func TestRetryPingRetriesTransientErrors(t *testing.T) {
	var attempts atomic.Int32

	ok := retryPing(t.Context(), nil, testExampleBaseURL, func(context.Context) error {
		current := attempts.Add(1)
		if current < 3 {
			return errors.New("temporary failure")
		}

		return nil
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

	ok := retryPing(t.Context(), nil, testExampleBaseURL, func(context.Context) error {
		attempts.Add(1)

		return &PingError{Err: errors.New("bad request")}
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

	ok := retryPing(ctx, nil, testExampleBaseURL, func(context.Context) error {
		attempt := attempts.Add(1)
		if attempt == 1 {
			cancel()
		}

		return errors.New("temporary failure")
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

	ok := retryPing(t.Context(), nil, testExampleBaseURL, func(context.Context) error {
		attempts.Add(1)

		return errors.New("temporary failure")
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

func TestPingProbesSameEndpointEveryCall(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		paths []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()

		paths = append(paths, r.URL.Path)
		mu.Unlock()

		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer server.Close()

	client := NewAPIClient(server.URL, "", WithTransport(transportHTTP1))

	if !client.Ping(t.Context()) {
		t.Fatal("first Ping() = false, want true")
	}

	mu.Lock()

	firstPaths := make([]string, len(paths))
	copy(firstPaths, paths)

	paths = nil
	mu.Unlock()

	if len(firstPaths) != 1 || firstPaths[0] != PathReady {
		t.Fatalf("first call paths = %v, want [/ready]", firstPaths)
	}

	if !client.Ping(t.Context()) {
		t.Fatal("second Ping() = false, want true")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(paths) != 1 || paths[0] != PathReady {
		t.Fatalf("second call paths = %v, want [/ready]", paths)
	}
}

func TestPingConcurrentCallsProbeOncePerCall(t *testing.T) {
	t.Parallel()

	var probeCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCount.Add(1)

		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer server.Close()

	client := NewAPIClient(server.URL, "", WithTransport(transportHTTP1))

	if !client.Ping(t.Context()) {
		t.Fatal("seed Ping() = false, want true")
	}

	probeCount.Store(0)

	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() {
			if !client.Ping(t.Context()) {
				t.Error("concurrent Ping() = false, want true")
			}
		})
	}

	wg.Wait()

	if count := probeCount.Load(); count != 20 {
		t.Fatalf("probe count = %d, want 20 (one GET /ready per successful Ping)", count)
	}
}
