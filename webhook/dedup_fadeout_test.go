package webhook

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	legacyDedupWarning   = "legacy stateless deduplicator"
	nonceFallbackWarning = "nonce cache falls back to the message dedup backend"
	senderHorizonWarning = "not shorter than the shortest wait before the sender's last retry"
	enqueueWindowWarning = "not shorter than the dedup pending TTL"
	dedupTTLReachWarning = "shorter than the arrival of the sender's last retransmission"
	pendingStaysWarning  = "the reservation stays pending"
)

type setOnceTestDeduplicator struct {
	memoryStatefulDeduplicator
}

func (d *setOnceTestDeduplicator) SetOnceNonce() {}

func TestLegacyStatelessDeduplicatorIsWarned(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newReleasableTestDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); !strings.Contains(got, legacyDedupWarning) {
		t.Fatalf("logs = %q, want a legacy stateless deduplicator warning", got)
	}
}

func TestStatefulDeduplicatorIsNotWarnedAsLegacy(t *testing.T) {
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

	if got := logs.String(); strings.Contains(got, legacyDedupWarning) {
		t.Fatalf("logs = %q, want no legacy warning for a stateful backend", got)
	}
}

// message dedup은 non-durable 경로에만 존재하므로 durable admitter 배포에는 이 경고가
// 해당하지 않는다. 무조건 경고하면 소비자가 경고 전체를 무시하게 된다.
func TestDurableAdmissionSuppressesLegacyDeduplicatorWarning(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := mustNewDurableHandler(
		t,
		&recordingAdmitter{},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newReleasableTestDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, legacyDedupWarning) {
		t.Fatalf("logs = %q, want no legacy warning in durable admission mode", got)
	}
}

func TestNoopDeduplicatorIsNotWarnedAsLegacy(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, legacyDedupWarning) {
		t.Fatalf("logs = %q, want no legacy warning when no deduplicator is configured", got)
	}
}

func TestSetOnceNonceStoreSuppressesImplicitFallbackWarning(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(&setOnceTestDeduplicator{memoryStatefulDeduplicator: *newMemoryStatefulDeduplicator()}),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, nonceFallbackWarning) {
		t.Fatalf("logs = %q, want no fallback warning for a declared set-once store", got)
	}
}

// 기본값 5s가 "요청값"으로 보고되면 DedupTTL이 5s 미만인 모든 소비자가 부르지도 않은
// 옵션에 대한 clamp 경고를 받는다.
func TestPendingTTLClampWarningIsNotAttributedToDefaults(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupTTL(2*time.Second),
	)
	defer closeHandler(t, handler)

	if got := handler.options.DedupPendingTTL; got != 2*time.Second {
		t.Fatalf("DedupPendingTTL = %v, want it clamped to the 2s DedupTTL", got)
	}
	if got := logs.String(); strings.Contains(got, "pending TTL exceeds the committed TTL") {
		t.Fatalf("logs = %q, want no clamp warning when WithDedupPendingTTL was never called", got)
	}
}

func TestServeHTTPDegradedReserveCommitsOrphanedReservation(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryStatefulDeduplicator()
	dedup.reserveErr = errors.New("dedup response lost")
	dedup.reserveErrAfterWrite = true

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

	recorder := serveDedupRequest(t, handler, "mid-orphan")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("fail-open admission did not dispatch the message")
	}

	if got := dedup.commitsSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-orphan}"}) {
		t.Fatalf("commits = %v, want the orphaned reservation reclaimed by Commit", got)
	}
	if got := logs.String(); !strings.Contains(got, "webhook dedup degraded") {
		t.Fatalf("logs = %q, want the degraded reserve to stay visible", got)
	}

	dedup.reserveErr = nil
	dedup.reserveErrAfterWrite = false

	duplicate := serveDedupRequest(t, handler, "mid-orphan")
	assertResponseCode(t, duplicate.Code, http.StatusOK)
	select {
	case msg := <-capture.msgCh:
		t.Fatalf("committed duplicate was dispatched again: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServeHTTPDegradedReserveReleasesOrphanedReservationOnEnqueueFailure(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
	dedup.reserveErr = errors.New("dedup response lost")
	dedup.reserveErrAfterWrite = true

	handler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 1)})
	closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-orphan-release")
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if got := dedup.releasesSnapshot(); !slices.Equal(got, []string{"iris:msg:{mid-orphan-release}"}) {
		t.Fatalf("releases = %v, want the orphaned reservation released", got)
	}
	if dedup.has("iris:msg:{mid-orphan-release}") {
		t.Fatal("orphaned reservation survived; the retransmission would be rejected with 503 until it expires")
	}
}

// 예약이 서버에 닿지 않은 것이 확실한 backend(빈 token)는 정리 시도 자체가 없어야 한다.
func TestServeHTTPDegradedReserveWithoutTokenSkipsReclaim(t *testing.T) {
	t.Parallel()

	dedup := newMemoryStatefulDeduplicator()
	dedup.reserveErr = errors.New("dedup backend down")

	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newStatefulHandler(t, dedup, capture)
	defer closeHandler(t, handler)

	recorder := serveDedupRequest(t, handler, "mid-no-token")
	assertResponseCode(t, recorder.Code, http.StatusOK)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("fail-open admission did not dispatch the message")
	}
	if got := dedup.commitsSnapshot(); len(got) != 0 {
		t.Fatalf("commits = %v, want none when no reservation could have landed", got)
	}
	if got := dedup.releasesSnapshot(); len(got) != 0 {
		t.Fatalf("releases = %v, want none when no reservation could have landed", got)
	}
}

func TestPendingTTLAgainstSenderFinalRetryWaitFloor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		pendingTTL time.Duration
		wantWarn   bool
	}{
		{name: "at the floor", pendingTTL: senderFinalRetryWaitFloor, wantWarn: true},
		{name: "one ms below the floor", pendingTTL: senderFinalRetryWaitFloor - time.Millisecond},
		{name: "past the last retry wait but under the full horizon", pendingTTL: 15 * time.Second, wantWarn: true},
		{name: "the default", pendingTTL: defaultDedupPendingTTL},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			logs := &lockedBuffer{}
			handler := NewHandler(
				t.Context(),
				"token",
				&captureHandler{msgCh: make(chan *Message, 1)},
				slog.New(slog.NewTextHandler(logs, nil)),
				WithDeduplicator(newMemoryStatefulDeduplicator()),
				WithNonceCache(newMemoryNonceCache()),
				WithDedupPendingTTL(testCase.pendingTTL),
			)
			defer closeHandler(t, handler)

			if got := handler.options.DedupPendingTTL; got != testCase.pendingTTL {
				t.Fatalf("DedupPendingTTL = %v, want %v kept unclamped", got, testCase.pendingTTL)
			}
			if got := strings.Contains(logs.String(), senderHorizonWarning); got != testCase.wantWarn {
				t.Fatalf(
					"warned = %t for pendingTTL %v, want %t (floor %v)\nlogs = %q",
					got, testCase.pendingTTL, testCase.wantWarn, senderFinalRetryWaitFloor, logs.String(),
				)
			}
		})
	}
}

func TestSenderHorizonConstantsMatchTheIrisRuntimeProfile(t *testing.T) {
	t.Parallel()

	const rationale = "this constant encodes an Iris runtime value that cannot be derived from this repository, " +
		"and every other assertion compares the sender horizon symbolically. " +
		"Without this literal anchor, lowering it silently shrinks senderRetransmitReachCeiling and lets a " +
		"WithDedupTTL that is actually too short pass the startup warn"

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "senderAttemptTimeout", got: senderAttemptTimeout, want: 125 * time.Second},
		{name: "senderBackoffCeiling", got: senderBackoffCeiling, want: 37200 * time.Millisecond},
		{name: "senderFinalRetryWaitFloor", got: senderFinalRetryWaitFloor, want: 12800 * time.Millisecond},
		{name: "senderRetransmitReachCeiling", got: senderRetransmitReachCeiling, want: 662200 * time.Millisecond},
	}

	for _, testCase := range cases {
		if testCase.got != testCase.want {
			t.Errorf("%s = %v, want %v; %s", testCase.name, testCase.got, testCase.want, rationale)
		}
	}

	if senderMaxAttempts != 6 {
		t.Errorf("senderMaxAttempts = %d, want 6; %s", senderMaxAttempts, rationale)
	}
}

func TestDefaultPendingTTLIsBelowSenderFinalRetryWaitFloor(t *testing.T) {
	t.Parallel()

	if defaultDedupPendingTTL >= senderFinalRetryWaitFloor {
		t.Fatalf(
			"defaultDedupPendingTTL = %v, must expire before the sender's last retry arrives (%v)",
			defaultDedupPendingTTL,
			senderFinalRetryWaitFloor,
		)
	}
}

func TestDefaultDedupTTLCoversTheSenderRetransmitReach(t *testing.T) {
	t.Parallel()

	if DefaultDedupTTL <= senderRetransmitReachCeiling {
		t.Fatalf(
			"DefaultDedupTTL = %v, must outlive the last retransmission (%v); otherwise an attempt that timed out after the message was already committed comes back to an expired key and is processed twice",
			DefaultDedupTTL,
			senderRetransmitReachCeiling,
		)
	}
}

func TestDedupTTLShorterThanTheSenderRetransmitReachIsWarned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		opts     []HandlerOption
		wantWarn bool
	}{
		{
			name: "one ms below the reach",
			opts: []HandlerOption{
				WithDeduplicator(newMemoryStatefulDeduplicator()),
				WithDedupTTL(senderRetransmitReachCeiling - time.Millisecond),
			},
			wantWarn: true,
		},
		{
			name: "exactly at the reach",
			opts: []HandlerOption{
				WithDeduplicator(newMemoryStatefulDeduplicator()),
				WithDedupTTL(senderRetransmitReachCeiling),
			},
		},
		{
			name: "the default",
			opts: []HandlerOption{WithDeduplicator(newMemoryStatefulDeduplicator())},
		},
		{
			name: "legacy stateless backend keyed by the same DedupTTL",
			opts: []HandlerOption{
				WithDeduplicator(newReleasableTestDeduplicator()),
				WithDedupTTL(time.Minute),
			},
			wantWarn: true,
		},
		{
			name: "no deduplicator stores no key",
			opts: []HandlerOption{WithDedupTTL(time.Minute)},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			logs := &lockedBuffer{}
			opts := append([]HandlerOption{WithNonceCache(newMemoryNonceCache())}, testCase.opts...)
			handler := NewHandler(
				t.Context(),
				"token",
				&captureHandler{msgCh: make(chan *Message, 1)},
				slog.New(slog.NewTextHandler(logs, nil)),
				opts...,
			)
			defer closeHandler(t, handler)

			if got := strings.Contains(logs.String(), dedupTTLReachWarning); got != testCase.wantWarn {
				t.Fatalf(
					"warned = %t, want %t (reach %v)\nlogs = %q",
					got, testCase.wantWarn, senderRetransmitReachCeiling, logs.String(),
				)
			}
		})
	}
}

func TestDurableAdmissionSuppressesTheDedupTTLReachWarning(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := mustNewDurableHandler(
		t,
		&recordingAdmitter{},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupTTL(time.Minute),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, dedupTTLReachWarning) {
		t.Fatalf("logs = %q, want no dedup TTL warning: durable admission never reaches handleDedupKey", got)
	}
}

func TestDedupPendingWindowGuardCountsBothDedupRoundTrips(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupTimeout(2*time.Second),
		WithEnqueueTimeout(time.Second),
		WithDedupPendingTTL(4*time.Second),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); !strings.Contains(got, enqueueWindowWarning) {
		t.Fatalf(
			"logs = %q, want the pending window warning: reserve and commit each get their own DedupTimeout, so the real budget is 1s+2*2s=5s against a 4s pending TTL",
			got,
		)
	}
}

// durable admitter는 handleDedupKey를 거치지 않으므로 예약 창 자체가 없다. 존재하지 않는
// 창에 대한 경고는 소비자가 경고 전체를 무시하게 만든다.
func TestDurableAdmissionSuppressesReservationWindowWarnings(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := mustNewDurableHandler(
		t,
		&recordingAdmitter{},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newMemoryStatefulDeduplicator()),
		WithNonceCache(newMemoryNonceCache()),
		WithEnqueueTimeout(6*time.Second),
		WithDedupPendingTTL(45*time.Second),
	)
	defer closeHandler(t, handler)

	got := logs.String()
	if strings.Contains(got, enqueueWindowWarning) {
		t.Fatalf("logs = %q, want no enqueue window warning in durable admission mode", got)
	}
	if strings.Contains(got, senderHorizonWarning) {
		t.Fatalf("logs = %q, want no sender horizon warning in durable admission mode", got)
	}
}

// legacy stateless backend도 명시 주입이 없으면 nonce cache로 재사용된다. 계약이 가장 덜
// 알려진 이 조합에 경고가 없으면 replay 보호가 조용히 fail-open된다.
func TestLegacyBackendImplicitNonceFallbackIsWarned(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(newReleasableTestDeduplicator()),
	)
	defer closeHandler(t, handler)

	if handler.nonceCache != Deduplicator(handler.dedup) {
		t.Fatal("legacy backend was not reused as the nonce cache; this test no longer covers the fallback")
	}
	if got := logs.String(); !strings.Contains(got, nonceFallbackWarning) {
		t.Fatalf("logs = %q, want a nonce fallback warning for a legacy stateless backend", got)
	}
}

func TestNoopDeduplicatorGetsNoNonceFallbackWarning(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
	)
	defer closeHandler(t, handler)

	if got := logs.String(); strings.Contains(got, nonceFallbackWarning) {
		t.Fatalf("logs = %q, want no fallback warning when the dedup backend is Noop", got)
	}
}

type pendingObservingMetrics struct {
	NoopMetrics

	pendingRejected atomic.Int32
}

func (m *pendingObservingMetrics) ObserveDedupPendingRejected() { m.pendingRejected.Add(1) }

// Metrics를 구현만 한 소비자는 이 마커가 없으면 pending 503을 어떤 metric에도 노출할 수
// 없다. Metrics 인터페이스에 메서드를 더하면 기존 구현이 전부 깨지므로 마커로 분리한다.
func TestDedupPendingObserverReceivesPendingRejections(t *testing.T) {
	t.Parallel()

	metrics := &pendingObservingMetrics{}
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

	handler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 4)}, WithMetrics(metrics))
	defer closeHandler(t, handler)
	defer close(gate)

	first := httptest.NewRecorder()
	firstRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-observer"))
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

	concurrent := serveDedupRequest(t, handler, "mid-observer")
	assertResponseCode(t, concurrent.Code, http.StatusServiceUnavailable)

	gate <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}

	if got := metrics.pendingRejected.Load(); got != 1 {
		t.Fatalf("ObserveDedupPendingRejected calls = %d, want 1", got)
	}
	if got := handler.Diagnostics().DedupPendingRejected; got != 1 {
		t.Fatalf("DedupPendingRejected = %d, want 1", got)
	}
}

func TestMetricsWithoutPendingObserverStillServesPending503(t *testing.T) {
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

	handler := newStatefulHandler(t, dedup, &captureHandler{msgCh: make(chan *Message, 4)}, WithMetrics(metrics))
	defer closeHandler(t, handler)
	defer close(gate)

	if handler.dedupPendingObserver != nil {
		t.Fatal("mockMetrics must not satisfy DedupPendingObserver; the marker would not be optional")
	}

	first := httptest.NewRecorder()
	firstRequest := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-no-observer"))
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

	concurrent := serveDedupRequest(t, handler, "mid-no-observer")
	assertResponseCode(t, concurrent.Code, http.StatusServiceUnavailable)

	gate <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}

	if got := handler.Diagnostics().DedupPendingRejected; got != 1 {
		t.Fatalf("DedupPendingRejected = %d, want 1", got)
	}
}

// 예약이 애초에 없을 수 있는 경우와 확실히 pending으로 남은 경우는 재전송 결과가 반대다.
func TestOrphanedCommitFailureDoesNotClaimThePendingOutcome(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryStatefulDeduplicator()
	dedup.commitErr = errors.New("commit transport blip")
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	handler.commitDedupReservation(t.Context(), dedupReservation{
		key: "iris:msg:{mid-orphan-commit}", token: "token-1", stateful: true, orphaned: true,
	})

	got := logs.String()
	if strings.Contains(got, pendingStaysWarning) {
		t.Fatalf("logs = %q, want no claim that the reservation stays pending; it may never have been stored", got)
	}
	if !strings.Contains(got, "degraded reserve") {
		t.Fatalf("logs = %q, want the degraded-reserve wording for an orphaned commit failure", got)
	}
}

func TestNonOrphanedCommitFailureKeepsThePendingOutcome(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	dedup := newMemoryStatefulDeduplicator()
	dedup.commitErr = errors.New("commit transport blip")
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.New(slog.NewTextHandler(logs, nil)),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	handler.commitDedupReservation(t.Context(), dedupReservation{
		key: "iris:msg:{mid-owned-commit}", token: "token-1", stateful: true,
	})

	if got := logs.String(); !strings.Contains(got, pendingStaysWarning) {
		t.Fatalf("logs = %q, want the pending-stays warning for an owned reservation", got)
	}
}
