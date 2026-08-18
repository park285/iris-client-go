package webhook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type messageDeduplicatorEntry struct {
	token     string
	committed bool
	expiresAt time.Time
}

type memoryMessageDeduplicator struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]messageDeduplicatorEntry
	tokens  atomic.Uint64

	reserveErr error
	// 예약이 저장소에 기록된 뒤 응답만 유실된 경우. token은 살아 있는 예약을 가리킨다.
	reserveErrAfterWrite bool
	commitErr            error
	releaseErr           error

	transientCommitFailures int

	afterReserve func(key string, state DedupState)

	reserveTTLs []time.Duration
	commitTTLs  []time.Duration
	commitCalls int
	commits     []string
	releases    []string
}

type pendingObserverMetrics struct {
	NoopMetrics
	count atomic.Uint64
}

func (m *pendingObserverMetrics) ObserveDedupPendingRejected() {
	m.count.Add(1)
}

const enqueueWindowWarning = "not shorter than the dedup pending TTL"

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

var _ MessageDeduplicator = (*memoryMessageDeduplicator)(nil)

func newMemoryMessageDeduplicator() *memoryMessageDeduplicator {
	return &memoryMessageDeduplicator{
		now:     time.Now,
		entries: make(map[string]messageDeduplicatorEntry),
	}
}

func (d *memoryMessageDeduplicator) Reserve(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, DedupState, error) {
	if err := ctx.Err(); err != nil {
		return "", DedupStateReserved, err
	}

	token, state, hook, err := d.reserve(key, ttl)
	if err != nil {
		return token, DedupStateReserved, err
	}
	if hook != nil {
		hook(key, state)
	}

	return token, state, nil
}

func (d *memoryMessageDeduplicator) reserve(
	key string,
	ttl time.Duration,
) (string, DedupState, func(string, DedupState), error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.reserveErr != nil && !d.reserveErrAfterWrite {
		return "", DedupStateReserved, nil, d.reserveErr
	}
	d.reserveTTLs = append(d.reserveTTLs, ttl)

	if entry, ok := d.entries[key]; ok && entry.expiresAt.After(d.now()) {
		if entry.committed {
			return "", DedupStateCommitted, d.afterReserve, nil
		}

		return "", DedupStatePending, d.afterReserve, nil
	}

	token := fmt.Sprintf("token-%d", d.tokens.Add(1))
	d.entries[key] = messageDeduplicatorEntry{token: token, expiresAt: d.now().Add(ttl)}

	if d.reserveErr != nil {
		return token, DedupStateReserved, nil, d.reserveErr
	}

	return token, DedupStateReserved, d.afterReserve, nil
}

func (d *memoryMessageDeduplicator) Commit(ctx context.Context, key, token string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.commitCalls++
	if d.transientCommitFailures > 0 {
		d.transientCommitFailures--

		return errors.New("commit backend blip")
	}
	if d.commitErr != nil {
		return d.commitErr
	}
	d.commitTTLs = append(d.commitTTLs, ttl)

	entry, ok := d.entries[key]
	if !ok || entry.committed || entry.token != token {
		return fmt.Errorf("commit %s: %w", key, ErrDedupReservationLost)
	}

	d.entries[key] = messageDeduplicatorEntry{token: token, committed: true, expiresAt: d.now().Add(ttl)}
	d.commits = append(d.commits, key)

	return nil
}

func (d *memoryMessageDeduplicator) ReleaseReservation(ctx context.Context, key, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.releaseErr != nil {
		return d.releaseErr
	}

	entry, ok := d.entries[key]
	if !ok || entry.committed || entry.token != token {
		return fmt.Errorf("release %s: %w", key, ErrDedupReservationLost)
	}

	delete(d.entries, key)
	d.releases = append(d.releases, key)

	return nil
}

func (d *memoryMessageDeduplicator) has(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, ok := d.entries[key]

	return ok
}

func (d *memoryMessageDeduplicator) ttlSnapshot() ([]time.Duration, []time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.reserveTTLs), slices.Clone(d.commitTTLs)
}

func (d *memoryMessageDeduplicator) commitCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.commitCalls
}

func (d *memoryMessageDeduplicator) commitsSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.commits)
}

func (d *memoryMessageDeduplicator) releasesSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.releases)
}

func newMessageDedupHandler(
	t *testing.T,
	dedup MessageDeduplicator,
	handler MessageHandler,
	opts ...HandlerOption,
) *Handler {
	t.Helper()

	merged := []HandlerOption{WithMessageDeduplicator(dedup), WithNonceStore(newMemoryNonceCache())}
	merged = append(merged, opts...)

	return newTestHandler(t.Context(), "token", handler, slog.Default(), merged...)
}

func serveDedupRequest(t *testing.T, handler *Handler, messageID string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newValidRequest(t, t.Context(), validJSONBodyWithMessageID(messageID)))

	return recorder
}

func TestServeHTTPStatefulConcurrentPendingDuplicateGets503(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := newMemoryMessageDeduplicator()
	reserved := make(chan struct{}, 1)
	gate := make(chan struct{})
	dedup.afterReserve = func(_ string, state DedupState) {
		if state != DedupStateReserved {
			return
		}
		select {
		case reserved <- struct{}{}:
		default:
		}
		<-gate
	}

	capture := &captureHandler{msgCh: make(chan *Message, 4)}
	handler := newMessageDedupHandler(t, dedup, capture, WithMetrics(metrics))
	defer closeHandler(t, handler)
	defer close(gate)

	first := httptest.NewRecorder()
	firstRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-race"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(first, firstRequest)
	}()

	select {
	case <-reserved:
	case <-time.After(time.Second):
		t.Fatal("first request did not reserve the dedup key")
	}

	concurrent := serveDedupRequest(t, handler, "mid-race")
	assertResponseCode(t, concurrent.Code, http.StatusServiceUnavailable)
	if got := dedup.commitsSnapshot(); len(got) != 0 {
		t.Fatalf("commits = %v, want none while the first request is still pending", got)
	}

	gate <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}
	assertResponseCode(t, first.Code, http.StatusOK)

	if got := dedup.commitsSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-race}"}) {
		t.Fatalf("commits = %v, want exactly one commit after successful enqueue", got)
	}

	committed := serveDedupRequest(t, handler, "mid-race")
	assertResponseCode(t, committed.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("admitted message was not dispatched")
	}
	select {
	case msg := <-capture.msgCh:
		t.Fatalf("duplicate reached the handler: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 3, accepted: 1, duplicate: 1})

	if got := handler.DedupPendingRejectedCount(); got != 1 {
		t.Fatalf("DedupPendingRejectedCount() = %d, want 1", got)
	}
}

func TestMessageDedupPendingObserverReceivesRejection(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	dedup.entries["iris:msg:{mid-pending-observer}"] = messageDeduplicatorEntry{
		token:     "foreign-owner",
		expiresAt: time.Now().Add(time.Minute),
	}
	metrics := &pendingObserverMetrics{}
	handler := newMessageDedupHandler(
		t,
		dedup,
		&captureHandler{msgCh: make(chan *Message, 1)},
		WithMetrics(metrics),
	)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-pending-observer")
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if got := metrics.count.Load(); got != 1 {
		t.Fatalf("pending observer count = %d, want 1", got)
	}
}

func TestServeHTTPStatefulClosedHandlerReleasesReservationForRetransmit(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	handler := newMessageDedupHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
	closeHandler(t, handler)

	rejected := serveDedupRequest(t, handler, "mid-closed")
	assertResponseCode(t, rejected.Code, http.StatusServiceUnavailable)

	if dedup.has("iris:msg:{mid-closed}") {
		t.Fatal("reservation survived enqueue failure; retransmit would be absorbed")
	}
	if got := dedup.releasesSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-closed}"}) {
		t.Fatalf("releases = %v, want exactly one token-bound release", got)
	}

	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	retryHandler := newMessageDedupHandler(t, dedup, capture)
	defer closeHandler(t, retryHandler)

	retry := serveDedupRequest(t, retryHandler, "mid-closed")
	assertResponseCode(t, retry.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("retransmit after 503 was absorbed as duplicate")
	}

	duplicate := serveDedupRequest(t, retryHandler, "mid-closed")
	assertResponseCode(t, duplicate.Code, http.StatusOK)

	if got := dedup.commitsSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-closed}"}) {
		t.Fatalf("commits = %v, want exactly one commit", got)
	}
}

func TestServeHTTPStatefulQueueFullReleasesReservationForRetransmit(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	worker := &gatedCaptureHandler{
		started: make(chan struct{}, 1),
		gate:    make(chan struct{}),
		msgs:    make(chan *Message, 8),
	}
	handler := newMessageDedupHandler(
		t,
		dedup,
		worker,
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
		recorder := serveDedupRequest(t, handler, fmt.Sprintf("mid-fill-%d", i))
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

	overflow := serveDedupRequest(t, handler, "mid-overflow")
	assertResponseCode(t, overflow.Code, http.StatusServiceUnavailable)

	if dedup.has("iris:msg:{mid-overflow}") {
		t.Fatal("reservation survived queue-full rejection")
	}
	if got := dedup.releasesSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-overflow}"}) {
		t.Fatalf("releases = %v, want exactly one release", got)
	}

	close(worker.gate)
	for i := range 3 {
		select {
		case <-worker.msgs:
		case <-time.After(time.Second):
			t.Fatalf("queued message %d was not drained", i)
		}
	}

	retry := serveDedupRequest(t, handler, "mid-overflow")
	assertResponseCode(t, retry.Code, http.StatusOK)

	select {
	case <-worker.msgs:
	case <-time.After(time.Second):
		t.Fatal("retransmit after queue-full 503 was absorbed as duplicate")
	}
}

func TestServeHTTPStatefulRequestCancelReleasesReservation(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	requestCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dedup.afterReserve = func(_ string, state DedupState) {
		if state == DedupStateReserved {
			cancel()
		}
	}

	handler := newMessageDedupHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newValidRequest(t, requestCtx, validJSONBodyWithMessageID("mid-cancel")))
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if dedup.has("iris:msg:{mid-cancel}") {
		t.Fatal("reservation survived request cancellation; release must not inherit the cancelled context")
	}
	if got := dedup.releasesSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-cancel}"}) {
		t.Fatalf("releases = %v, want exactly one release", got)
	}
}

func TestServeHTTPStatefulReleaseFailureKeepsReservationPending(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryMessageDeduplicator()
	dedup.releaseErr = errors.New("release backend down")

	handler := newTestHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	rejected := serveDedupRequest(t, handler, "mid-release-fail")
	assertResponseCode(t, rejected.Code, http.StatusServiceUnavailable)

	if !dedup.has("iris:msg:{mid-release-fail}") {
		t.Fatal("reservation disappeared even though release failed")
	}
	if got := logs.String(); !strings.Contains(got, "webhook dedup release failed") {
		t.Fatalf("logs = %q, want a dedup release failure warning", got)
	}

	retryHandler := newMessageDedupHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
	defer closeHandler(t, retryHandler)

	retry := serveDedupRequest(t, retryHandler, "mid-release-fail")
	assertResponseCode(t, retry.Code, http.StatusServiceUnavailable)
}

func TestServeHTTPStatefulCommitFailureStillReturns200(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryMessageDeduplicator()
	dedup.commitErr = errors.New("commit backend down")
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := newTestHandler(
		t.Context(),
		"token",
		capture,
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-commit-fail")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("admitted message was not dispatched")
	}
	if got := logs.String(); !strings.Contains(got, "webhook dedup commit failed") {
		t.Fatalf("logs = %q, want a dedup commit failure warning", got)
	}
}

func TestServeHTTPMessageDedupReserveErrorFailsClosed(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	dedup.reserveErr = errors.New("dedup backend down")
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newMessageDedupHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-degraded")
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	select {
	case msg := <-capture.msgCh:
		t.Fatalf("reserve error dispatched message: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}
	if got := dedup.releasesSnapshot(); len(got) != 0 {
		t.Fatalf("releases = %v, want none when no reservation was taken", got)
	}
	if got := dedup.commitsSnapshot(); len(got) != 0 {
		t.Fatalf("commits = %v, want none when no reservation was taken", got)
	}
}

func TestServeHTTPMessageDedupAmbiguousReserveReleasesOwnTokenAndFailsClosed(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	dedup.reserveErr = errors.New("reserve response lost")
	dedup.reserveErrAfterWrite = true
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newMessageDedupHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-ambiguous-reserve")
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	select {
	case msg := <-capture.msgCh:
		t.Fatalf("ambiguous reserve dispatched message: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}
	if got := dedup.releasesSnapshot(); len(got) != 1 {
		t.Fatalf("releases = %v, want one owner-token cleanup", got)
	}
	if dedup.has("iris:msg:{mid-ambiguous-reserve}") {
		t.Fatal("owner reservation survived conditional cleanup")
	}
}

func TestStatefulReserveUsesPendingTTLAndCommitUsesDedupTTL(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newMessageDedupHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-ttl")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	reserveTTLs, commitTTLs := dedup.ttlSnapshot()
	if len(reserveTTLs) != 1 || reserveTTLs[0] != defaultDedupPendingTTL {
		t.Fatalf("reserve TTLs = %v, want [%v]", reserveTTLs, defaultDedupPendingTTL)
	}
	if len(commitTTLs) != 1 || commitTTLs[0] != DefaultDedupTTL {
		t.Fatalf("commit TTLs = %v, want [%v]", commitTTLs, DefaultDedupTTL)
	}
	if defaultDedupPendingTTL >= senderFinalRetryWaitFloor {
		t.Fatalf(
			"defaultDedupPendingTTL = %v, must expire before the sender's last retry arrives (%v)",
			defaultDedupPendingTTL,
			senderFinalRetryWaitFloor,
		)
	}
}

func TestNormalizeDedupPendingTTLClampsToDedupTTL(t *testing.T) {
	t.Parallel()

	got := normalizeDedupPendingTTL(time.Minute, 10*time.Second)
	if got != 10*time.Second {
		t.Fatalf("normalizeDedupPendingTTL() = %v, want it clamped to DedupTTL 10s", got)
	}

	defaulted := normalizeDedupPendingTTL(0, DefaultDedupTTL)
	if defaulted != defaultDedupPendingTTL {
		t.Fatalf("normalizeDedupPendingTTL() = %v, want %v", defaulted, defaultDedupPendingTTL)
	}
}

func TestStatefulPendingTTLInversionIsWarned(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryMessageDeduplicator()
	handler := newTestHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
		WithDedupTTL(10*time.Second),
		WithDedupPendingTTL(time.Minute),
	)
	defer closeHandler(t, handler)

	if handler.dedupPendingTTL != 10*time.Second {
		t.Fatalf("dedupPendingTTL = %v, want clamped 10s", handler.dedupPendingTTL)
	}
	if got := logs.String(); !strings.Contains(got, "pending TTL exceeds the committed TTL") {
		t.Fatalf("logs = %q, want a pending TTL inversion warning", got)
	}
}

func TestStatefulEnqueueTimeoutExceedingPendingTTLIsWarned(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := newTestHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(newMemoryMessageDeduplicator()),
		WithNonceStore(newMemoryNonceCache()),
		WithDedupPendingTTL(20*time.Millisecond),
		WithEnqueueTimeout(50*time.Millisecond),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); !strings.Contains(got, enqueueWindowWarning) {
		t.Fatalf("logs = %q, want an enqueue timeout inversion warning", got)
	}
}

func TestStatefulDefaultInFlightWindowIsShorterThanPendingTTL(t *testing.T) {
	t.Parallel()

	if defaultEnqueueTimeout+2*defaultDedupTimeout >= defaultDedupPendingTTL {
		t.Fatalf(
			"defaultEnqueueTimeout(%v) + two defaultDedupTimeout round trips(%v) must expire before the reservation (%v) so Commit is still valid",
			defaultEnqueueTimeout,
			defaultDedupTimeout,
			defaultDedupPendingTTL,
		)
	}
}

func TestStatefulNoInversionWarningForDefaultTimeouts(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := newTestHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(newMemoryMessageDeduplicator()),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, "is not shorter than the dedup pending TTL") {
		t.Fatalf("logs = %q, want no inversion warning for the default timeout combination", got)
	}
}

func TestServeHTTPStatefulTransientCommitFailureIsRetried(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryMessageDeduplicator()
	dedup.transientCommitFailures = 2
	capture := &captureHandler{msgCh: make(chan *Message, 2)}

	handler := newTestHandler(
		t.Context(),
		"token",
		capture,
		slog.New(slog.NewTextHandler(logs, nil)),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-commit-blip")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	if got := dedup.commitCallCount(); got != 3 {
		t.Fatalf("commit calls = %d, want 3 (two transient failures then success)", got)
	}
	if got := dedup.commitsSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-commit-blip}"}) {
		t.Fatalf("commits = %v, want the reservation committed after retries", got)
	}
	if got := logs.String(); strings.Contains(got, "webhook dedup commit failed") {
		t.Fatalf("logs = %q, want no commit failure warning after a successful retry", got)
	}

	duplicate := serveDedupRequest(t, handler, "mid-commit-blip")
	assertResponseCode(t, duplicate.Code, http.StatusOK)
}

func TestServeHTTPStatefulCommitDoesNotRetryLostReservation(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	dedup.commitErr = fmt.Errorf("commit: %w", ErrDedupReservationLost)
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newMessageDedupHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-lost")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	if got := dedup.commitCallCount(); got != 1 {
		t.Fatalf("commit calls = %d, want 1; a foreign owner's key must not be overwritten by retries", got)
	}
}

func TestMemoryMessageDeduplicatorRejectsForeignToken(t *testing.T) {
	t.Parallel()

	dedup := newMemoryMessageDeduplicator()
	key := "iris:msg:{mid-owner}"

	token, state, err := dedup.Reserve(t.Context(), key, time.Minute)
	if err != nil || state != DedupStateReserved || token == "" {
		t.Fatalf("Reserve() = %q, %v, %v, want a token with DedupStateReserved", token, state, err)
	}

	if err := dedup.ReleaseReservation(t.Context(), key, "token-foreign"); !errors.Is(err, ErrDedupReservationLost) {
		t.Fatalf("ReleaseReservation(foreign token) error = %v, want ErrDedupReservationLost", err)
	}
	if !dedup.has(key) {
		t.Fatal("foreign token deleted another owner's reservation")
	}
	if err := dedup.Commit(t.Context(), key, "token-foreign", time.Minute); !errors.Is(err, ErrDedupReservationLost) {
		t.Fatalf("Commit(foreign token) error = %v, want ErrDedupReservationLost", err)
	}

	if _, state, err := dedup.Reserve(t.Context(), key, time.Minute); err != nil || state != DedupStatePending {
		t.Fatalf("Reserve() state = %v, %v, want DedupStatePending", state, err)
	}

	if err := dedup.ReleaseReservation(t.Context(), key, token); err != nil {
		t.Fatalf("ReleaseReservation(owner token) error = %v, want nil", err)
	}
	if dedup.has(key) {
		t.Fatal("owner release did not remove the reservation")
	}
}
