package webhook

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type messageDeduplicatorCall struct {
	key   string
	token string
	ttl   time.Duration
}

type statefulTestDeduplicator struct {
	mu           sync.Mutex
	state        DedupState
	token        string
	reserveCalls []messageDeduplicatorCall
	commitCalls  []messageDeduplicatorCall
	releaseCalls []messageDeduplicatorCall
}

func (d *statefulTestDeduplicator) Reserve(_ context.Context, key string, ttl time.Duration) (string, DedupState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reserveCalls = append(d.reserveCalls, messageDeduplicatorCall{key: key, ttl: ttl})
	return d.token, d.state, nil
}

func (d *statefulTestDeduplicator) Commit(_ context.Context, key, token string, ttl time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.commitCalls = append(d.commitCalls, messageDeduplicatorCall{key: key, token: token, ttl: ttl})
	d.state = DedupStateCommitted
	d.token = ""
	return nil
}

func (d *statefulTestDeduplicator) ReleaseReservation(_ context.Context, key, token string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.releaseCalls = append(d.releaseCalls, messageDeduplicatorCall{key: key, token: token})
	d.state = DedupStateReserved
	return nil
}

func (d *statefulTestDeduplicator) snapshots() ([]messageDeduplicatorCall, []messageDeduplicatorCall, []messageDeduplicatorCall) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]messageDeduplicatorCall(nil), d.reserveCalls...),
		append([]messageDeduplicatorCall(nil), d.commitCalls...),
		append([]messageDeduplicatorCall(nil), d.releaseCalls...)
}

func TestServeHTTPStatefulDedupCommitsOnlyAfterSuccessfulEnqueue(t *testing.T) {
	t.Parallel()

	dedup := &statefulTestDeduplicator{state: DedupStateReserved, token: "owner-1"}
	capture := &captureHandler{msgCh: make(chan *Message, 2)}
	handler := newTestHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-stateful")))
	assertResponseCode(t, first.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("reserved request was not enqueued")
	}

	reserveCalls, commitCalls, releaseCalls := dedup.snapshots()
	if len(reserveCalls) != 1 || reserveCalls[0].key != "iris:msg:{mid-stateful}" {
		t.Fatalf("reserve calls = %#v", reserveCalls)
	}
	if len(commitCalls) != 1 || commitCalls[0].token != "owner-1" || commitCalls[0].key != reserveCalls[0].key {
		t.Fatalf("commit calls = %#v", commitCalls)
	}
	if len(releaseCalls) != 0 {
		t.Fatalf("release calls = %#v, want none", releaseCalls)
	}

	duplicate := httptest.NewRecorder()
	handler.ServeHTTP(duplicate, newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-stateful")))
	assertResponseCode(t, duplicate.Code, http.StatusOK)

	select {
	case message := <-capture.msgCh:
		t.Fatalf("committed duplicate was enqueued: %#v", message)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServeHTTPStatefulDedupPendingReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	dedup := &statefulTestDeduplicator{state: DedupStatePending}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newTestHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-pending")))
	assertResponseCode(t, response.Code, http.StatusServiceUnavailable)

	select {
	case message := <-capture.msgCh:
		t.Fatalf("pending duplicate was enqueued: %#v", message)
	case <-time.After(50 * time.Millisecond):
	}

	_, commitCalls, releaseCalls := dedup.snapshots()
	if len(commitCalls) != 0 || len(releaseCalls) != 0 {
		t.Fatalf("pending transition calls commit=%#v release=%#v", commitCalls, releaseCalls)
	}
}

func TestServeHTTPStatefulDedupReleasesOwnedReservationOnEnqueueFailure(t *testing.T) {
	t.Parallel()

	dedup := &statefulTestDeduplicator{state: DedupStateReserved, token: "owner-2"}
	handler := newTestHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-release")))
	assertResponseCode(t, response.Code, http.StatusServiceUnavailable)

	_, commitCalls, releaseCalls := dedup.snapshots()
	if len(commitCalls) != 0 {
		t.Fatalf("commit calls = %#v, want none", commitCalls)
	}
	if len(releaseCalls) != 1 || releaseCalls[0].key != "iris:msg:{mid-release}" || releaseCalls[0].token != "owner-2" {
		t.Fatalf("release calls = %#v", releaseCalls)
	}
}
