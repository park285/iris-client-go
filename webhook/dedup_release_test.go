package webhook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

type releasableTestDeduplicator struct {
	mu       sync.Mutex
	err      error
	reserved map[string]struct{}
	released []string
}

func newReleasableTestDeduplicator() *releasableTestDeduplicator {
	return &releasableTestDeduplicator{reserved: make(map[string]struct{})}
}

func (d *releasableTestDeduplicator) IsDuplicate(_ context.Context, key string, _ time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.err != nil {
		return false, d.err
	}
	if _, ok := d.reserved[key]; ok {
		return true, nil
	}
	d.reserved[key] = struct{}{}

	return false, nil
}

func (d *releasableTestDeduplicator) Release(_ context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.reserved, key)
	d.released = append(d.released, key)

	return nil
}

func (d *releasableTestDeduplicator) isReserved(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, ok := d.reserved[key]

	return ok
}

func (d *releasableTestDeduplicator) releasedSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.released)
}

type gatedCaptureHandler struct {
	started chan struct{}
	gate    chan struct{}
	msgs    chan *Message
}

func (h *gatedCaptureHandler) HandleMessage(_ context.Context, msg *Message) {
	select {
	case h.started <- struct{}{}:
	default:
	}

	<-h.gate
	h.msgs <- msg
}

func TestServeHTTPQueueFullReleasesDedupReservationForRetry(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := newReleasableTestDeduplicator()
	worker := &gatedCaptureHandler{
		started: make(chan struct{}, 1),
		gate:    make(chan struct{}),
		msgs:    make(chan *Message, 8),
	}
	handler := NewHandler(
		t.Context(),
		"token",
		worker,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithEnqueueTimeout(10*time.Millisecond),
	)
	defer closeHandler(t, handler)
	defer func() {
		select {
		case <-worker.gate:
		default:
			close(worker.gate)
		}
	}()

	for i := range 3 {
		recorder := httptest.NewRecorder()
		request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID(fmt.Sprintf("mid-fill-%d", i)))
		handler.ServeHTTP(recorder, request)
		assertResponseCode(t, recorder.Code, http.StatusOK)
		if i == 0 {
			select {
			case <-worker.started:
			case <-time.After(time.Second):
				t.Fatal("worker did not start")
			}
		}
	}
	eventually(t, time.Second, func() bool {
		return handler.sched.depth.Load() >= 2
	})
	if got := dedup.releasedSnapshot(); len(got) != 0 {
		t.Fatalf("released = %v, want none on successful enqueue", got)
	}

	overflow := httptest.NewRecorder()
	overflowRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-retry"))
	handler.ServeHTTP(overflow, overflowRequest)
	assertResponseCode(t, overflow.Code, http.StatusServiceUnavailable)

	if dedup.isReserved("iris:msg:{mid-retry}") {
		t.Fatal("dedup reservation survived enqueue failure; retry would be absorbed as duplicate")
	}
	if got := dedup.releasedSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-retry}"}) {
		t.Fatalf("released = %v, want exactly one release of iris:msg:{mid-retry}", got)
	}

	close(worker.gate)
	for i := range 3 {
		select {
		case <-worker.msgs:
		case <-time.After(time.Second):
			t.Fatalf("queued message %d was not drained", i)
		}
	}

	retry := httptest.NewRecorder()
	retryRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-retry"))
	handler.ServeHTTP(retry, retryRequest)
	assertResponseCode(t, retry.Code, http.StatusOK)

	select {
	case msg := <-worker.msgs:
		if msg == nil || msg.JSON == nil || msg.JSON.MessageID != "mid-retry" {
			t.Fatalf("retried message = %#v, want messageID mid-retry", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("retry after queue-full 503 was absorbed as duplicate")
	}

	if got := dedup.releasedSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-retry}"}) {
		t.Fatalf("released = %v, want unchanged after successful retry enqueue", got)
	}
	assertMetricCounts(t, metrics, metricCounts{requests: 5, accepted: 4, enqueueFailure: 1})
}

func TestServeHTTPEnqueueFailureAfterCloseReleasesDedupReservation(t *testing.T) {
	t.Parallel()

	dedup := newReleasableTestDeduplicator()
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-closed"))
	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if dedup.isReserved("iris:msg:{mid-closed}") {
		t.Fatal("dedup reservation survived shutdown enqueue failure; retry would be absorbed as duplicate")
	}

	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	retryHandler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, retryHandler)

	retry := httptest.NewRecorder()
	retryRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-closed"))
	retryHandler.ServeHTTP(retry, retryRequest)
	assertResponseCode(t, retry.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("retry after shutdown 503 was absorbed as duplicate")
	}

	duplicated := httptest.NewRecorder()
	duplicatedRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-closed"))
	retryHandler.ServeHTTP(duplicated, duplicatedRequest)
	assertResponseCode(t, duplicated.Code, http.StatusOK)

	if got := dedup.releasedSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-closed}"}) {
		t.Fatalf("released = %v, want unchanged after duplicate absorption", got)
	}
	select {
	case msg := <-capture.msgCh:
		t.Fatalf("duplicate request reached handler: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServeHTTPDegradedDedupEnqueueFailureDoesNotRelease(t *testing.T) {
	t.Parallel()

	dedup := newReleasableTestDeduplicator()
	dedup.err = errors.New("dedup backend down")
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-degraded"))
	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if got := dedup.releasedSnapshot(); len(got) != 0 {
		t.Fatalf("released = %v, want none when reservation ownership is unknown", got)
	}
}

func TestMemoryNonceCacheReleaseAllowsReuse(t *testing.T) {
	t.Parallel()

	cache := newMemoryNonceCache()
	if duplicate, err := cache.IsDuplicate(t.Context(), "key-1", time.Minute); err != nil || duplicate {
		t.Fatalf("first IsDuplicate() = %v, %v, want false, nil", duplicate, err)
	}
	if duplicate, err := cache.IsDuplicate(t.Context(), "key-1", time.Minute); err != nil || !duplicate {
		t.Fatalf("second IsDuplicate() = %v, %v, want true, nil", duplicate, err)
	}

	if err := cache.Release(t.Context(), "key-1"); err != nil {
		t.Fatalf("Release() error = %v, want nil", err)
	}

	if duplicate, err := cache.IsDuplicate(t.Context(), "key-1", time.Minute); err != nil || duplicate {
		t.Fatalf("IsDuplicate() after Release = %v, %v, want false, nil", duplicate, err)
	}
}

func TestServeHTTPEnqueueFailureWithoutReleaseSupportKeepsFailOpen(t *testing.T) {
	t.Parallel()

	dedup := &mockDeduplicator{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-no-release"))
	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if calls := dedup.snapshot(); len(calls) != 1 {
		t.Fatalf("dedup calls = %d, want 1", len(calls))
	}
}
