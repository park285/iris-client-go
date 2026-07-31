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

type statefulDedupEntry struct {
	token     string
	committed bool
	expiresAt time.Time
}

type memoryStatefulDeduplicator struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]statefulDedupEntry
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

var _ StatefulDeduplicator = (*memoryStatefulDeduplicator)(nil)

func newMemoryStatefulDeduplicator() *memoryStatefulDeduplicator {
	return &memoryStatefulDeduplicator{
		now:     time.Now,
		entries: make(map[string]statefulDedupEntry),
	}
}

func (d *memoryStatefulDeduplicator) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.entries[key]; ok && entry.expiresAt.After(d.now()) {
		return true, nil
	}
	d.entries[key] = statefulDedupEntry{committed: true, expiresAt: d.now().Add(ttl)}

	return false, nil
}

func (d *memoryStatefulDeduplicator) Reserve(
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

func (d *memoryStatefulDeduplicator) reserve(
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
	d.entries[key] = statefulDedupEntry{token: token, expiresAt: d.now().Add(ttl)}

	if d.reserveErr != nil {
		return token, DedupStateReserved, nil, d.reserveErr
	}

	return token, DedupStateReserved, d.afterReserve, nil
}

func (d *memoryStatefulDeduplicator) Commit(ctx context.Context, key, token string, ttl time.Duration) error {
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

	d.entries[key] = statefulDedupEntry{token: token, committed: true, expiresAt: d.now().Add(ttl)}
	d.commits = append(d.commits, key)

	return nil
}

func (d *memoryStatefulDeduplicator) ReleaseReservation(ctx context.Context, key, token string) error {
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

func (d *memoryStatefulDeduplicator) has(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, ok := d.entries[key]

	return ok
}

func (d *memoryStatefulDeduplicator) ttlSnapshot() ([]time.Duration, []time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.reserveTTLs), slices.Clone(d.commitTTLs)
}

func (d *memoryStatefulDeduplicator) commitCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.commitCalls
}

func (d *memoryStatefulDeduplicator) commitsSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.commits)
}

func (d *memoryStatefulDeduplicator) releasesSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.releases)
}

func newStatefulHandler(
	t *testing.T,
	dedup Deduplicator,
	handler MessageHandler,
	opts ...HandlerOption,
) *Handler {
	t.Helper()

	merged := []HandlerOption{WithDeduplicator(dedup), WithNonceCache(newMemoryNonceCache())}
	merged = append(merged, opts...)

	return NewHandler(t.Context(), "token", handler, slog.Default(), merged...)
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
	dedup := newMemoryStatefulDeduplicator()
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
	handler := newStatefulHandler(t, dedup, capture, WithMetrics(metrics))
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

func TestServeHTTPStatefulClosedHandlerReleasesReservationForRetransmit(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
	handler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
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
	retryHandler := newStatefulHandler(t, dedup, capture)
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

	dedup := newMemoryStatefulDeduplicator()
	worker := &gatedCaptureHandler{
		started: make(chan struct{}, 1),
		gate:    make(chan struct{}),
		msgs:    make(chan *Message, 8),
	}
	handler := newStatefulHandler(
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

	dedup := newMemoryStatefulDeduplicator()
	requestCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dedup.afterReserve = func(_ string, state DedupState) {
		if state == DedupStateReserved {
			cancel()
		}
	}

	handler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
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
	dedup := newMemoryStatefulDeduplicator()
	dedup.releaseErr = errors.New("release backend down")

	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
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

	retryHandler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
	defer closeHandler(t, retryHandler)

	retry := serveDedupRequest(t, retryHandler, "mid-release-fail")
	assertResponseCode(t, retry.Code, http.StatusServiceUnavailable)
}

func TestServeHTTPStatefulCommitFailureStillReturns200(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryStatefulDeduplicator()
	dedup.commitErr = errors.New("commit backend down")
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
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

func TestServeHTTPStatefulReserveErrorFailsOpen(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
	dedup.reserveErr = errors.New("dedup backend down")
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newStatefulHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-degraded")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("fail-open admission did not dispatch the message")
	}
	if got := dedup.releasesSnapshot(); len(got) != 0 {
		t.Fatalf("releases = %v, want none when no reservation was taken", got)
	}
	if got := dedup.commitsSnapshot(); len(got) != 0 {
		t.Fatalf("commits = %v, want none when no reservation was taken", got)
	}
}

func TestStatefulReserveUsesPendingTTLAndCommitUsesDedupTTL(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newStatefulHandler(t, dedup, capture)
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
	dedup := newMemoryStatefulDeduplicator()
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
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
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
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
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, "is not shorter than the dedup pending TTL") {
		t.Fatalf("logs = %q, want no inversion warning for the default timeout combination", got)
	}
}

func TestStatefulImplicitNonceCacheFallbackIsWarned(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); !strings.Contains(got, nonceFallbackWarning) {
		t.Fatalf("logs = %q, want an implicit nonce cache fallback warning", got)
	}
}

func TestServeHTTPStatefulTransientCommitFailureIsRetried(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryStatefulDeduplicator()
	dedup.transientCommitFailures = 2
	capture := &captureHandler{msgCh: make(chan *Message, 2)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
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

	dedup := newMemoryStatefulDeduplicator()
	dedup.commitErr = fmt.Errorf("commit: %w", ErrDedupReservationLost)
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newStatefulHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-lost")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	if got := dedup.commitCallCount(); got != 1 {
		t.Fatalf("commit calls = %d, want 1; a foreign owner's key must not be overwritten by retries", got)
	}
}

func TestMemoryStatefulDeduplicatorRejectsForeignToken(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
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

func TestServeHTTPLegacyDeduplicatorAbsorbsDuplicateWith200(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := newReleasableTestDeduplicator()
	capture := &captureHandler{msgCh: make(chan *Message, 2)}
	handler := newStatefulHandler(t, dedup, capture, WithMetrics(metrics))
	defer closeHandler(t, handler)

	if handler.statefulDedup != nil {
		t.Fatal("legacy deduplicator must not be promoted to the stateful contract")
	}

	first := serveDedupRequest(t, handler, "mid-legacy")
	assertResponseCode(t, first.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("first legacy request was not dispatched")
	}

	duplicate := serveDedupRequest(t, handler, "mid-legacy")
	assertResponseCode(t, duplicate.Code, http.StatusOK)

	if got := dedup.releasedSnapshot(); len(got) != 0 {
		t.Fatalf("released = %v, want none on the legacy success path", got)
	}
	assertMetricCounts(t, metrics, metricCounts{requests: 2, accepted: 1, duplicate: 1})
}
