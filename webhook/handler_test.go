package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

type mockMetrics struct {
	requests        atomic.Int32
	unauthorized    atomic.Int32
	badRequest      atomic.Int32
	duplicate       atomic.Int32
	enqueueFailure  atomic.Int32
	accepted        atomic.Int32
	decodeLatency   atomic.Int64
	dedupLatency    atomic.Int64
	enqueueWait     atomic.Int64
	queueDepth      atomic.Int32
	queueDepthCalls atomic.Int32
	handlerDuration atomic.Int64
}

type metricCounts struct {
	requests       int32
	unauthorized   int32
	badRequest     int32
	duplicate      int32
	enqueueFailure int32
	accepted       int32
}

var testHMACNonce atomic.Uint64

func signHandlerTestRequest(t *testing.T, request *http.Request, secret string, body string) {
	t.Helper()

	nonce := fmt.Sprintf("handler-test-%d", testHMACNonce.Add(1))
	signWebhookTestRequest(t, request, secret, time.Now(), nonce, []byte(body))
}

func (m *mockMetrics) ObserveRequest() {
	m.requests.Add(1)
}

func (m *mockMetrics) ObserveUnauthorized() {
	m.unauthorized.Add(1)
}

func (m *mockMetrics) ObserveBadRequest() {
	m.badRequest.Add(1)
}

func (m *mockMetrics) ObserveDuplicate() {
	m.duplicate.Add(1)
}

func (m *mockMetrics) ObserveEnqueueFailure() {
	m.enqueueFailure.Add(1)
}

func (m *mockMetrics) ObserveAccepted() {
	m.accepted.Add(1)
}

func (m *mockMetrics) ObserveDecodeLatency(d time.Duration) {
	m.decodeLatency.Add(int64(d))
}

func (m *mockMetrics) ObserveDedupLatency(d time.Duration) {
	m.dedupLatency.Add(int64(d))
}

func (m *mockMetrics) ObserveEnqueueWait(d time.Duration) {
	m.enqueueWait.Add(int64(d))
}

func (m *mockMetrics) ObserveQueueDepth(depth int) {
	m.queueDepth.Store(int32(depth))
	m.queueDepthCalls.Add(1)
}

func (m *mockMetrics) ObserveHandlerDuration(d time.Duration) {
	m.handlerDuration.Add(int64(d))
}

type captureHandler struct {
	msgCh chan *Message
}

func (h *captureHandler) HandleMessage(_ context.Context, msg *Message) {
	select {
	case h.msgCh <- msg:
	default:
	}
}

type blockingHandler struct {
	started chan struct{}
	block   chan struct{}
}

func (h *blockingHandler) HandleMessage(_ context.Context, _ *Message) {
	select {
	case h.started <- struct{}{}:
	default:
	}

	<-h.block
}

type timeoutAwareHandler struct {
	done chan struct{}
}

func (h *timeoutAwareHandler) HandleMessage(ctx context.Context, _ *Message) {
	<-ctx.Done()
	close(h.done)
}

type closeAwareHandler struct {
	started chan struct{}
	done    chan error
}

func (h *closeAwareHandler) HandleMessage(ctx context.Context, _ *Message) {
	close(h.started)
	<-ctx.Done()
	h.done <- ctx.Err()
}

type countingBlockingHandler struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (h *countingBlockingHandler) HandleMessage(_ context.Context, _ *Message) {
	call := h.calls.Add(1)
	if call == 1 {
		select {
		case h.started <- struct{}{}:
		default:
		}

		<-h.release
	}
}

type panicHandler struct {
	calls atomic.Int32
}

func (h *panicHandler) HandleMessage(_ context.Context, _ *Message) {
	h.calls.Add(1)
	panic("sensitive handler panic payload")
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type dedupCall struct {
	key string
	ttl time.Duration
}

type mockDeduplicator struct {
	mu        sync.Mutex
	duplicate bool
	err       error
	calls     []dedupCall
}

func (d *mockDeduplicator) IsDuplicate(_ context.Context, key string, ttl time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls = append(d.calls, dedupCall{key: key, ttl: ttl})

	return d.duplicate, d.err
}

func (d *mockDeduplicator) snapshot() []dedupCall {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]dedupCall, len(d.calls))
	copy(result, d.calls)

	return result
}

type serveHTTPValidationCase struct {
	name        string
	method      string
	protoMajor  int
	token       string
	headerToken string
	contentType string
	body        string
	opts        []HandlerOption
	wantStatus  int
	wantMetrics metricCounts
}

type recordingAdmitter struct {
	calls int
	msg   *Message
	err   error
}

type closeAwareAdmitter struct {
	started chan struct{}
	done    chan error
}

func (a *closeAwareAdmitter) AdmitMessage(ctx context.Context, _ *Message) error {
	close(a.started)
	<-ctx.Done()
	a.done <- ctx.Err()

	return ctx.Err()
}

func (a *recordingAdmitter) AdmitMessage(_ context.Context, msg *Message) error {
	a.calls++
	a.msg = msg

	return a.err
}

func TestServeHTTPDurableAdmissionCommitsBeforeOKAndSkipsMemoryQueue(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := NewHandler(t.Context(), "token", capture, slog.Default(),
		WithDurableAdmission(admitter),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeAfterDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := acceptedCaseRequest(t)
	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)
	if admitter.calls != 1 || admitter.msg == nil {
		t.Fatalf("admission = calls:%d msg:%#v, want one committed message", admitter.calls, admitter.msg)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup calls = %#v, want none because durable unique key owns idempotency", calls)
	}
	if handler.sched != nil || handler.taskPool != nil {
		t.Fatalf("durable handler created memory queue: scheduler=%T taskPool=%T", handler.sched, handler.taskPool)
	}
	select {
	case msg := <-capture.msgCh:
		t.Fatalf("message bypassed inbox: %#v", msg)
	default:
	}
}

func TestServeHTTPDurableAdmissionFailureReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{err: errors.New("commit failed")}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}
}

func TestDurableAdmissionDoesNotPromoteV1HeaderMessageIDIntoPayload(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	defer closeHandler(t, handler)
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "header-message-id")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)
	if admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != "" {
		t.Fatalf("admitted message = %#v, want unsigned v1 header excluded from payload identity", admitter.msg)
	}
}

func TestDurableAdmissionCloseContextCancelsCommitAfterGrace(t *testing.T) {
	t.Parallel()

	admitter := &closeAwareAdmitter{started: make(chan struct{}), done: make(chan error, 1)}
	handler := NewHandler(context.Background(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default(), WithDurableAdmission(admitter), WithNonceCache(newMemoryNonceCache()))
	request := acceptedCaseRequest(t)
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-admitter.started:
	case <-time.After(time.Second):
		t.Fatal("durable admission did not start")
	}
	closeCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := handler.CloseContext(closeCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseContext() error = %v, want context.Canceled", err)
	}
	select {
	case err := <-admitter.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("admission context error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("durable admission was not canceled")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("ServeHTTP did not return after admission cancellation")
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("Close() after forced cancellation error = %v", err)
	}
}

func TestServeHTTPValidation(t *testing.T) {
	t.Parallel()

	for _, tt := range serveHTTPValidationCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runServeHTTPValidationCase(t, tt)
		})
	}
}

func TestServeHTTPAcceptedBuildsMessageAndUsesDedup(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := newAcceptedCaseHandler(t, metrics, dedup, capture)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := acceptedCaseRequest(t)
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)
	assertAcceptedMessage(t, capture)
	assertAcceptedDedup(t, dedup)
	assertMetricCounts(t, metrics, metricCounts{requests: 1, accepted: 1})
}

func TestServeHTTPDuplicateReturnsOKWithoutEnqueue(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-1"))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-1")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	select {
	case msg := <-capture.msgCh:
		t.Fatalf("unexpected message: %#v", msg)
	case <-time.After(50 * time.Millisecond):
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, duplicate: 1})
}

func TestServeHTTPUnsupportedMediaTypeSkipsDedup(t *testing.T) {
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
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := validJSONBody()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-unsupported")
	signHandlerTestRequest(t, request, "token", body)

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusUnsupportedMediaType)

	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup calls = %d, want 0", len(calls))
	}
}

func TestServeHTTPBeforeDecodeModeRejectsMalformedWithoutDedup(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeBeforeDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := "{invalid-json"
	request := newV2IdentityRequest(t, body, "mid-early")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup calls = %d, want 0 for rejected body", len(calls))
	}
	assertMetricCounts(t, metrics, metricCounts{requests: 1, badRequest: 1})
}

func TestServeHTTPDedupErrorDegradesToAccepted(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{err: errors.New("boom")}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-1"))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-1")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, accepted: 1})
}

func TestServeHTTPEnqueueFailureAfterClose(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithMetrics(metrics),
	)
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, enqueueFailure: 1})
}

func TestServeHTTPLatencyMetricsRecorded(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithMessageID("mid-lat"))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "mid-lat")

	handler.ServeHTTP(recorder, request)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}

	eventually(t, time.Second, func() bool {
		return metrics.handlerDuration.Load() > 0
	})

	if metrics.decodeLatency.Load() <= 0 {
		t.Fatal("decode latency not recorded")
	}

	if metrics.dedupLatency.Load() <= 0 {
		t.Fatal("dedup latency not recorded")
	}
}

func TestServeHTTPBackpressureReturns503(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	blocker := &blockingHandler{
		started: make(chan struct{}, 1),
		block:   make(chan struct{}),
	}

	handler := NewHandler(
		t.Context(),
		"token",
		blocker,
		slog.Default(),
		WithMetrics(metrics),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithEnqueueTimeout(10*time.Millisecond),
	)
	defer closeBlockingHandler(handler, blocker.block)

	// queueSize=2: worker에서 처리 중 1개 + dispatcher 대기 2개 = 3개까지 OK
	for i := range 3 {
		recorder := httptest.NewRecorder()
		request := newValidRequest(t, t.Context(), validJSONBody())
		request.Header.Set(HeaderIrisToken, "token")
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}

		if i == 0 {
			select {
			case <-blocker.started:
			case <-time.After(time.Second):
				t.Fatal("worker did not start")
			}
		}
	}

	fourth := httptest.NewRecorder()
	req4 := newValidRequest(t, t.Context(), validJSONBody())
	req4.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(fourth, req4)

	if fourth.Code != http.StatusServiceUnavailable {
		t.Fatalf("fourth status = %d, want %d", fourth.Code, http.StatusServiceUnavailable)
	}
}

func TestReceiveQueueSizeIncludesExecutionHandoff(t *testing.T) {
	t.Parallel()

	blocker := &blockingHandler{started: make(chan struct{}, 1), block: make(chan struct{})}
	handler := NewHandler(
		t.Context(),
		"token",
		blocker,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithOrderingMode(OrderingModeNone),
		WithEnqueueTimeout(20*time.Millisecond),
	)
	defer closeBlockingHandler(handler, blocker.block)

	for index := range 3 {
		recorder := httptest.NewRecorder()
		request := newValidRequest(t, t.Context(), validJSONBodyWithRoom(fmt.Sprintf("room-%d", index)))
		request.Header.Set(HeaderIrisToken, "token")
		handler.ServeHTTP(recorder, request)
		assertResponseCode(t, recorder.Code, http.StatusOK)
		if index == 0 {
			select {
			case <-blocker.started:
			case <-time.After(time.Second):
				t.Fatal("worker did not start")
			}
		}
	}

	overflow := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBodyWithRoom("room-overflow"))
	request.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(overflow, request)
	assertResponseCode(t, overflow.Code, http.StatusServiceUnavailable)
}

func TestServeHTTPBackpressureReservesCapacityForDifferentShard(t *testing.T) {
	t.Parallel()

	hotRoom, coldRoom := roomsForDifferentSchedulerShards(2)
	blocker := &blockingHandler{
		started: make(chan struct{}, 1),
		block:   make(chan struct{}),
	}

	handler := NewHandler(
		t.Context(),
		"token",
		blocker,
		slog.Default(),
		WithWorkerCount(2),
		WithQueueSize(2),
		WithEnqueueTimeout(10*time.Millisecond),
	)
	defer closeBlockingHandler(handler, blocker.block)

	for i := range 2 {
		recorder := httptest.NewRecorder()
		request := newValidRequest(t, t.Context(), validJSONBodyWithRoom(hotRoom))
		request.Header.Set(HeaderIrisToken, "token")
		handler.ServeHTTP(recorder, request)

		assertResponseCode(t, recorder.Code, http.StatusOK)

		if i == 0 {
			select {
			case <-blocker.started:
			case <-time.After(time.Second):
				t.Fatal("hot shard worker did not start")
			}
		}
	}

	hotOverflow := httptest.NewRecorder()
	hotRequest := newValidRequest(t, t.Context(), validJSONBodyWithRoom(hotRoom))
	hotRequest.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(hotOverflow, hotRequest)
	assertResponseCode(t, hotOverflow.Code, http.StatusServiceUnavailable)

	coldRecorder := httptest.NewRecorder()
	coldRequest := newValidRequest(t, t.Context(), validJSONBodyWithRoom(coldRoom))
	coldRequest.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(coldRecorder, coldRequest)
	assertResponseCode(t, coldRecorder.Code, http.StatusOK)
}

func TestServeHTTPBlockedEnqueueReturnsOnRequestContextCancel(t *testing.T) {
	t.Parallel()

	blocker, handler := newBackpressureFixture(t, time.Second)
	defer closeBlockingHandler(handler, blocker.block)

	recorder := httptest.NewRecorder()
	reqCtx, cancel := context.WithCancel(t.Context())
	request := newValidRequest(t, reqCtx, validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")

	done := beginServeHTTP(handler, recorder, request)
	assertServeHTTPStillBlocked(t, done)

	cancel()

	assertServeHTTPCompletes(t, done, 200*time.Millisecond)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
}

func TestServeHTTPBlockedEnqueueReturnsOnClose(t *testing.T) {
	t.Parallel()

	blocker, handler := newBackpressureFixture(t, time.Second)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")

	requestDone := beginServeHTTP(handler, recorder, request)
	assertServeHTTPStillBlocked(t, requestDone)

	closeDone := beginHandlerClose(handler)

	assertServeHTTPCompletes(t, requestDone, 200*time.Millisecond)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	assertCloseBlocks(t, closeDone)

	close(blocker.block)
	assertCloseCompletes(t, closeDone)
}

func TestDiagnosticsReportsConfiguredReceiveAndQueueRejections(t *testing.T) {
	t.Parallel()

	blocker, handler := newBackpressureFixture(t, 20*time.Millisecond)
	defer closeBlockingHandler(handler, blocker.block)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")

	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	diagnostics := handler.Diagnostics()
	if diagnostics.WorkersConfigured != 1 || diagnostics.QueueSize != 2 {
		t.Fatalf("Diagnostics() configured = %+v, want workers=1 queue=2", diagnostics)
	}
	if diagnostics.InFlight != 1 {
		t.Fatalf("Diagnostics().InFlight = %d, want 1", diagnostics.InFlight)
	}
	if diagnostics.EnqueueRejected != 1 || diagnostics.QueueFullCount != 1 {
		t.Fatalf("Diagnostics() rejected/full = %+v, want 1/1", diagnostics)
	}
}

func TestDiagnosticsPendingIncludesTaskWaitingForExecutionWorker(t *testing.T) {
	t.Parallel()

	blocker := &blockingHandler{started: make(chan struct{}, 1), block: make(chan struct{})}
	handler := NewHandler(
		t.Context(),
		"token",
		blocker,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithOrderingMode(OrderingModeNone),
	)
	defer closeBlockingHandler(handler, blocker.block)
	mustEnqueue(t, handler, webhookTask{msg: &Message{Room: "first"}}, "first")
	select {
	case <-blocker.started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	mustEnqueue(t, handler, webhookTask{msg: &Message{Room: "second"}}, "second")
	eventually(t, time.Second, func() bool { return handler.sched.depth.Load() == 1 })

	diagnostics := handler.Diagnostics()
	if diagnostics.Pending != 1 || diagnostics.InFlight != 1 {
		t.Fatalf("Diagnostics() pending/in-flight = %d/%d, want 1/1", diagnostics.Pending, diagnostics.InFlight)
	}
}

func TestDiagnosticsCountsHandlerTimeouts(t *testing.T) {
	t.Parallel()

	handlerImpl := &timeoutAwareHandler{done: make(chan struct{})}
	handler := NewHandler(
		t.Context(),
		"token",
		handlerImpl,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithHandlerTimeout(20*time.Millisecond),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(recorder, request)
	assertAcceptedResponse(t, recorder)

	select {
	case <-handlerImpl.done:
	case <-time.After(time.Second):
		t.Fatal("handler did not observe timeout")
	}

	eventually(t, time.Second, func() bool {
		return handler.Diagnostics().HandlerTimeouts == 1
	})
}

func TestStripeKey(t *testing.T) {
	t.Parallel()

	threadID := "thread-1"
	tests := []struct {
		name string
		msg  *Message
		want string
	}{
		{name: "nil message", want: ""},
		{name: "room only", msg: &Message{Room: " room-1 "}, want: "room-1"},
		{name: "room and thread", msg: &Message{Room: "room-1", JSON: &MessageJSON{ThreadID: &threadID}}, want: "room-1:thread-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stripeKey(tt.msg); got != tt.want {
				t.Fatalf("stripeKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloseDrainsQueueAndRejectsNewTasks(t *testing.T) {
	t.Parallel()

	worker, handler, task := newCloseDrainFixture(t)
	waitForWorkerStart(t, worker)

	done := beginHandlerClose(handler)
	assertCloseBlocks(t, done)
	close(worker.release)
	assertCloseCompletes(t, done)
	assertHandledCalls(t, worker.calls.Load(), 2)

	if err := handler.enqueue(task); !errors.Is(err, errClosed) {
		t.Fatalf("enqueue after close error = %v, want %v", err, errClosed)
	}
}

func TestHandlerOrderingNoneAllowsConcurrentSameKeyTasks(t *testing.T) {
	t.Parallel()

	worker := &countingBlockingHandler{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	handler := NewHandler(
		t.Context(),
		"token",
		worker,
		slog.Default(),
		WithWorkerCount(2),
		WithQueueSize(4),
		WithOrderingMode(OrderingModeNone),
	)
	defer closeBlockingHandler(handler, worker.release)

	threadID := "thread-1"
	task := webhookTask{msg: &Message{Room: "room-1", JSON: &MessageJSON{ThreadID: &threadID}}}
	mustEnqueue(t, handler, task, "first")
	waitForWorkerStart(t, worker)

	mustEnqueue(t, handler, task, "second")
	eventually(t, time.Second, func() bool {
		return worker.calls.Load() == 2
	})
}

func TestWithTaskPool_Injection(t *testing.T) {
	t.Parallel()

	pool := &recordingTaskPool{
		runTasks: true,
		submits:  make(chan func(), 1),
	}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithTaskPool(pool),
		WithWorkerCount(1),
		WithQueueSize(1),
	)
	defer closeHandler(t, handler)

	if handler.taskPool != pool {
		t.Fatal("handler taskPool was not set to injected pool")
	}
	if handler.ownsPool {
		t.Fatal("handler owns injected pool, want external ownership")
	}

	if err := handler.enqueue(webhookTask{msg: &Message{Msg: "msg"}}); err != nil {
		t.Fatalf("enqueue error = %v", err)
	}

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message through injected pool")
	}

	if got := pool.calls.Load(); got != 1 {
		t.Fatalf("SubmitWait calls = %d, want 1", got)
	}
}

func TestHandler_Close_OwnsPool(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(1),
	)

	pool, ok := handler.taskPool.(*internalPool)
	if !ok {
		t.Fatalf("taskPool = %T, want *internalPool", handler.taskPool)
	}
	if !handler.ownsPool {
		t.Fatal("handler ownsPool = false, want true for fallback pool")
	}

	closeHandler(t, handler)

	if !internalPoolClosed(pool) {
		t.Fatal("owned internal pool was not stopped")
	}
	if ok := pool.SubmitWait(func() {}); ok {
		t.Fatal("owned internal pool accepted task after Handler.Close")
	}
}

func TestHandlerInternalExecutionQueueIsUnbuffered(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(32),
	)
	defer closeHandler(t, handler)

	pool, ok := handler.taskPool.(*internalPool)
	if !ok {
		t.Fatalf("taskPool = %T, want *internalPool", handler.taskPool)
	}
	if got := cap(pool.queue); got != 0 {
		t.Fatalf("internal execution queue capacity = %d, want 0", got)
	}
	if handler.options.QueueSize != 32 {
		t.Fatalf("ordering queue size = %d, want 32", handler.options.QueueSize)
	}
}

func TestHandler_Close_InjectedPool(t *testing.T) {
	t.Parallel()

	pool := &recordingTaskPool{runTasks: true}
	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		WithTaskPool(pool),
		WithWorkerCount(1),
		WithQueueSize(1),
	)

	closeHandler(t, handler)

	if got := pool.stopCalls.Load(); got != 0 {
		t.Fatalf("injected pool StopAndWait calls = %d, want 0", got)
	}
}

func TestHandlerCloseContextCancelsInFlightAfterGraceExpires(t *testing.T) {
	t.Parallel()

	worker := &closeAwareHandler{started: make(chan struct{}), done: make(chan error, 1)}
	handler := NewHandler(
		context.Background(),
		"token",
		worker,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(1),
		WithHandlerTimeout(time.Minute),
	)
	mustEnqueue(t, handler, webhookTask{msg: &Message{Msg: "running"}}, "running")
	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	closeCtx, cancelClose := context.WithCancel(context.Background())
	cancelClose()
	if err := handler.CloseContext(closeCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseContext() error = %v, want context.Canceled", err)
	}
	select {
	case err := <-worker.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("worker context error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight handler was not canceled")
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("Close() after forced cancellation error = %v", err)
	}
}

func TestWorkerRecoversFromPanic(t *testing.T) {
	t.Parallel()

	worker := &panicHandler{}
	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	handler := NewHandler(
		t.Context(),
		"token",
		worker,
		logger,
		WithWorkerCount(1),
	)
	defer closeHandler(t, handler)

	if err := handler.enqueue(webhookTask{msg: &Message{Msg: "msg"}}); err != nil {
		t.Fatalf("enqueue error = %v", err)
	}

	eventually(t, time.Second, func() bool {
		return worker.calls.Load() == 1
	})
	eventually(t, time.Second, func() bool {
		return strings.Contains(logs.String(), `"msg":"webhook_scheduler_runner_panic_recovered"`)
	})
	logLine := logs.String()
	for _, token := range []string{`"panic_type":"string"`, `"stack":`} {
		if !strings.Contains(logLine, token) {
			t.Fatalf("panic recovery log missing %s: %s", token, logLine)
		}
	}
	if strings.Contains(logLine, "sensitive handler panic payload") {
		t.Fatalf("panic recovery log exposed panic payload: %s", logLine)
	}
}

func serveHTTPValidationCases() []serveHTTPValidationCase {
	cases := append([]serveHTTPValidationCase{}, basicServeHTTPValidationCases()...)

	cases = append(cases, bodyErrorServeHTTPValidationCases()...)

	return cases
}

func basicServeHTTPValidationCases() []serveHTTPValidationCase {
	cases := append([]serveHTTPValidationCase{}, methodAndProtocolValidationCases()...)

	cases = append(cases, authAndContentTypeValidationCases()...)
	cases = append(cases, invalidJSONValidationCase())

	return cases
}

func methodAndProtocolValidationCases() []serveHTTPValidationCase {
	return []serveHTTPValidationCase{
		{
			name:        "method not allowed",
			method:      http.MethodGet,
			wantStatus:  http.StatusMethodNotAllowed,
			wantMetrics: metricCounts{requests: 1},
		},
	}
}

func authAndContentTypeValidationCases() []serveHTTPValidationCase {
	return []serveHTTPValidationCase{
		{
			name:        "missing configured token",
			method:      http.MethodPost,
			protoMajor:  1,
			contentType: "application/json",
			body:        validJSONBody(),
			wantStatus:  http.StatusInternalServerError,
			wantMetrics: metricCounts{requests: 1},
		},
		{
			name:        "unauthorized",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "wrong",
			contentType: "application/json",
			body:        validJSONBody(),
			wantStatus:  http.StatusUnauthorized,
			wantMetrics: metricCounts{requests: 1, unauthorized: 1},
		},
		{
			name:        "missing content type",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			body:        validJSONBody(),
			wantStatus:  http.StatusUnsupportedMediaType,
			wantMetrics: metricCounts{requests: 1},
		},
		{
			name:        "invalid content type",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			contentType: "text/plain",
			body:        validJSONBody(),
			wantStatus:  http.StatusUnsupportedMediaType,
			wantMetrics: metricCounts{requests: 1},
		},
	}
}

func invalidJSONValidationCase() serveHTTPValidationCase {
	return serveHTTPValidationCase{
		name:        "invalid json",
		method:      http.MethodPost,
		protoMajor:  1,
		token:       "token",
		headerToken: "token",
		contentType: "application/json",
		body:        "{",
		wantStatus:  http.StatusBadRequest,
		wantMetrics: metricCounts{requests: 1, badRequest: 1},
	}
}

func bodyErrorServeHTTPValidationCases() []serveHTTPValidationCase {
	return []serveHTTPValidationCase{
		{
			name:        "invalid payload",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			contentType: "application/json",
			body:        `{"text":"","room":"room-1","userId":"user-1"}`,
			wantStatus:  http.StatusBadRequest,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
		{
			name:        "body too large",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			contentType: "application/json",
			body:        validJSONBody(),
			opts:        []HandlerOption{WithMaxBodyBytes(8)},
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
		{
			name:        "attachment too large",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			contentType: "application/json",
			body:        `{"text":"hello","room":"room-1","userId":"user-1","attachment":"` + strings.Repeat("x", 65537) + `"}`,
			wantStatus:  http.StatusBadRequest,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
	}
}

func runServeHTTPValidationCase(t *testing.T, tt serveHTTPValidationCase) {
	t.Helper()

	metrics := &mockMetrics{}

	handler := NewHandler(
		t.Context(),
		tt.token,
		&captureHandler{msgCh: make(chan *Message, 1)},
		slog.Default(),
		append([]HandlerOption{WithMetrics(metrics)}, tt.opts...)...,
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), tt.method, "/webhook/iris", strings.NewReader(tt.body))
	setRequestProtoMajor(request, tt.protoMajor)
	setRequestHeader(request, HeaderIrisToken, tt.headerToken)
	setRequestHeader(request, "Content-Type", tt.contentType)
	if tt.token != "" && tt.headerToken == strings.TrimSpace(tt.token) {
		signHandlerTestRequest(t, request, tt.headerToken, tt.body)
	}
	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, tt.wantStatus)
	assertMetricCounts(t, metrics, tt.wantMetrics)
}

func newAcceptedCaseHandler(t *testing.T, metrics *mockMetrics, dedup *mockDeduplicator, capture *captureHandler) *Handler {
	t.Helper()

	return NewHandler(
		t.Context(),
		" token ",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupTTL(90*time.Second),
	)
}

func acceptedCaseRequest(t *testing.T) *http.Request {
	t.Helper()

	body := `{"route":" default ","messageId":" msg-1 ","sourceLogId":1000000000001,"rawSourceLogId":1,"sourceGenerationId":1,"sourceAccountId":" 123456789 ","text":" hello ","room":" room-1 ","sender":" tester ","userId":" user-1 ","chatLogId":" chat-1 ","roomType":" OD ","roomLinkId":" room-link ","threadId":" 123 ","threadScope":2,"type":" 1 ","isMine":true,"origin":" WRITE ","attachment":"{\"url\":\"test\"}"}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, " msg-1 ")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	signHandlerTestRequest(t, request, "token", body)

	return request
}

func assertAcceptedResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	assertResponseCode(t, recorder.Code, http.StatusOK)
}

func assertAcceptedMessage(t *testing.T, capture *captureHandler) {
	t.Helper()

	threadScope := 2
	want := &Message{
		Msg:    " hello ",
		Room:   " room-1 ",
		Sender: ptrString("tester"),
		JSON: &MessageJSON{
			UserID:             " user-1 ",
			Message:            " hello ",
			ChatID:             " room-1 ",
			Type:               "1",
			Route:              "default",
			MessageID:          "msg-1",
			ChatLogID:          "chat-1",
			RoomType:           "OD",
			RoomLinkID:         "room-link",
			SourceLogID:        ptrInt64(1_000_000_000_001),
			RawSourceLogID:     ptrInt64(1),
			SourceGenerationID: ptrInt64(1),
			SourceAccountID:    "123456789",
			ThreadID:           ptrString("123"),
			ThreadScope:        &threadScope,
			IsMine:             ptrBool(true),
			Origin:             "WRITE",
			Attachment:         "{\"url\":\"test\"}",
		},
	}

	select {
	case got := <-capture.msgCh:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("message = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}
}

func TestServeHTTPAcceptedPreservesEventPayload(t *testing.T) {
	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newAcceptedCaseHandler(t, metrics, dedup, capture)
	defer closeHandler(t, handler)

	body := `{"messageId":"msg-event-1","text":"{\"type\":\"member_nickname_updated\"}","room":"room-a","sender":"iris-system","userId":"0","type":"member_nickname_updated","eventPayload":{"previousDisplayName":"alice","currentDisplayName":"alice2","createdAtMs":1778226335000}}`
	request := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/webhook/iris",
		strings.NewReader(body),
	)
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "msg-event-1")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)

	calls := dedup.snapshot()
	if len(calls) != 1 {
		t.Fatalf("dedup call count = %d, want %d", len(calls), 1)
	}

	if calls[0].key != "iris:msg:{msg-event-1}" {
		t.Fatalf("dedup key = %q, want %q", calls[0].key, "iris:msg:{msg-event-1}")
	}

	select {
	case got := <-capture.msgCh:
		if got == nil || got.JSON == nil {
			t.Fatalf("message = %#v, want payload-bearing message", got)
		}

		if got.JSON.Type != "member_nickname_updated" {
			t.Fatalf("Type = %q, want %q", got.JSON.Type, "member_nickname_updated")
		}

		var payload map[string]any
		if err := jsonx.Unmarshal(got.JSON.EventPayload, &payload); err != nil {
			t.Fatalf("Unmarshal(EventPayload) error = %v", err)
		}

		if payload["previousDisplayName"] != "alice" || payload["currentDisplayName"] != "alice2" {
			t.Fatalf("EventPayload = %#v, want nickname payload", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}
}

func TestServeHTTPAcceptedPreservesEventPayloadWithoutText(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := newAcceptedCaseHandler(t, metrics, dedup, capture)
	defer closeHandler(t, handler)

	body := `{"room":"room-a","sender":"iris-system","userId":"0","type":"member_nickname_updated","eventPayload":{"previousDisplayName":"alice","currentDisplayName":"alice2","createdAtMs":1778226335000}}`
	request := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/webhook/iris",
		strings.NewReader(body),
	)
	request.Header.Set(HeaderIrisToken, "token")
	request.Header.Set(HeaderIrisMessageID, "msg-event-no-text-1")
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)

	select {
	case got := <-capture.msgCh:
		if got == nil || got.JSON == nil {
			t.Fatalf("message = %#v, want payload-bearing message", got)
		}
		if got.Msg != "" {
			t.Fatalf("Msg = %q, want empty event marker text", got.Msg)
		}
		if got.JSON.Type != "member_nickname_updated" {
			t.Fatalf("Type = %q, want %q", got.JSON.Type, "member_nickname_updated")
		}
		var payload map[string]any
		if err := jsonx.Unmarshal(got.JSON.EventPayload, &payload); err != nil {
			t.Fatalf("Unmarshal(EventPayload) error = %v", err)
		}
		if payload["previousDisplayName"] != "alice" || payload["currentDisplayName"] != "alice2" {
			t.Fatalf("EventPayload = %#v, want nickname payload", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}
}

func TestBuildMessageJSON_DoesNotFallbackThreadIDFromChatLogID(t *testing.T) {
	got := buildMessageJSON(WebhookRequest{
		Text:       "hello",
		Room:       "room-1",
		UserID:     "user-1",
		ChatLogID:  "54321",
		RoomType:   "OD",
		RoomLinkID: "room-link",
		Type:       "2",
	})

	if got.ThreadID != nil {
		t.Fatalf("ThreadID = %v, want nil when webhook did not provide observed thread", *got.ThreadID)
	}
}

func TestBuildMessageJSONPreservesEventPayload(t *testing.T) {
	got := buildMessageJSON(WebhookRequest{
		Text:         `{"type":"member_nickname_updated"}`,
		Room:         "room-1",
		UserID:       "0",
		Type:         "member_nickname_updated",
		EventPayload: []byte(`{"previousDisplayName":"alice","currentDisplayName":"alice2"}`),
	})

	if string(got.EventPayload) != `{"previousDisplayName":"alice","currentDisplayName":"alice2"}` {
		t.Fatalf("EventPayload = %s, want raw payload", got.EventPayload)
	}
}

func TestBuildMessageJSONCopiesMentions(t *testing.T) {
	got := buildMessageJSON(WebhookRequest{
		Text:   "!누구 @카푸치노",
		Room:   "room-a",
		UserID: "user-1",
		Mentions: []WebhookMention{
			{UserID: "8691114094424718810", At: []int{4}, Len: 4},
		},
	})

	want := []WebhookMention{{UserID: "8691114094424718810", At: []int{4}, Len: 4}}
	if !reflect.DeepEqual(got.Mentions, want) {
		t.Fatalf("MessageJSON.Mentions = %#v, want %#v", got.Mentions, want)
	}
}

func assertAcceptedDedup(t *testing.T, dedup *mockDeduplicator) {
	t.Helper()

	calls := dedup.snapshot()
	if len(calls) != 1 {
		t.Fatalf("dedup call count = %d, want %d", len(calls), 1)
	}

	if calls[0].key != "iris:msg:{msg-1}" {
		t.Fatalf("dedup key = %q, want %q", calls[0].key, "iris:msg:{msg-1}")
	}

	if calls[0].ttl != 90*time.Second {
		t.Fatalf("dedup ttl = %v, want %v", calls[0].ttl, 90*time.Second)
	}
}

func newCloseDrainFixture(t *testing.T) (*countingBlockingHandler, *Handler, webhookTask) {
	t.Helper()

	worker := &countingBlockingHandler{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	handler := NewHandler(
		t.Context(),
		"token",
		worker,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithEnqueueTimeout(20*time.Millisecond),
	)
	task := webhookTask{msg: &Message{Msg: "msg"}}
	mustEnqueue(t, handler, task, "first")
	mustEnqueue(t, handler, task, "second")

	return worker, handler, task
}

func newBackpressureFixture(t *testing.T, enqueueTimeout time.Duration) (*blockingHandler, *Handler) {
	t.Helper()

	blocker := &blockingHandler{
		started: make(chan struct{}, 1),
		block:   make(chan struct{}),
	}
	handler := NewHandler(
		t.Context(),
		"token",
		blocker,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(2),
		WithEnqueueTimeout(enqueueTimeout),
	)

	task := webhookTask{msg: &Message{Msg: "msg"}}
	for i := range 3 {
		mustEnqueue(t, handler, task, "prefill")
		if i == 0 {
			select {
			case <-blocker.started:
			case <-time.After(time.Second):
				t.Fatal("worker did not start")
			}
		}
	}

	eventually(t, time.Second, func() bool {
		return handler.sched.depth.Load() >= 2
	})

	return blocker, handler
}

func mustEnqueue(t *testing.T, handler *Handler, task webhookTask, label string) {
	t.Helper()

	if err := handler.enqueue(task); err != nil {
		t.Fatalf("%s enqueue error = %v", label, err)
	}
}

func waitForWorkerStart(t *testing.T, worker *countingBlockingHandler) {
	t.Helper()

	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
}

func beginHandlerClose(handler *Handler) <-chan error {
	done := make(chan error, 1)

	go func() {
		done <- handler.Close()
	}()

	return done
}

func beginServeHTTP(handler *Handler, recorder *httptest.ResponseRecorder, request *http.Request) <-chan struct{} {
	done := make(chan struct{}, 1)

	go func() {
		handler.ServeHTTP(recorder, request)
		done <- struct{}{}
	}()

	return done
}

func assertCloseBlocks(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		t.Fatalf("Close returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func assertCloseCompletes(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return")
	}
}

func assertServeHTTPStillBlocked(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
		t.Fatal("ServeHTTP returned early")
	case <-time.After(50 * time.Millisecond):
	}
}

func assertServeHTTPCompletes(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("ServeHTTP did not return")
	}
}

func assertHandledCalls(t *testing.T, got, want int32) {
	t.Helper()

	if got != want {
		t.Fatalf("handled calls = %d, want %d", got, want)
	}
}

func assertMetricCounts(t *testing.T, got *mockMetrics, want metricCounts) {
	t.Helper()

	if got.requests.Load() != want.requests {
		t.Fatalf("requests = %d, want %d", got.requests.Load(), want.requests)
	}

	if got.unauthorized.Load() != want.unauthorized {
		t.Fatalf("unauthorized = %d, want %d", got.unauthorized.Load(), want.unauthorized)
	}

	if got.badRequest.Load() != want.badRequest {
		t.Fatalf("badRequest = %d, want %d", got.badRequest.Load(), want.badRequest)
	}

	if got.duplicate.Load() != want.duplicate {
		t.Fatalf("duplicate = %d, want %d", got.duplicate.Load(), want.duplicate)
	}

	if got.enqueueFailure.Load() != want.enqueueFailure {
		t.Fatalf("enqueueFailure = %d, want %d", got.enqueueFailure.Load(), want.enqueueFailure)
	}

	if got.accepted.Load() != want.accepted {
		t.Fatalf("accepted = %d, want %d", got.accepted.Load(), want.accepted)
	}
}

func assertResponseCode(t *testing.T, got, want int) {
	t.Helper()

	if got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func setRequestProtoMajor(request *http.Request, protoMajor int) {
	if protoMajor <= 0 {
		request.ProtoMajor = 1

		return
	}

	request.ProtoMajor = protoMajor
}

func setRequestHeader(request *http.Request, key, value string) {
	if value == "" {
		return
	}

	request.Header.Set(key, value)
}

func closeHandler(t *testing.T, handler *Handler) {
	t.Helper()

	if err := handler.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func validJSONBody() string {
	return `{"text":"hello","room":"room-1","sender":"tester","userId":"user-1"}`
}

func validJSONBodyWithMessageID(messageID string) string {
	return fmt.Sprintf(`{"messageId":%q,"text":"hello","room":"room-1","sender":"tester","userId":"user-1"}`, messageID)
}

func validJSONBodyWithRoom(room string) string {
	return fmt.Sprintf(`{"text":"hello","room":"%s","sender":"tester","userId":"user-1"}`, room)
}

func newValidRequest(t *testing.T, ctx context.Context, body string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	signHandlerTestRequest(t, request, "token", body)

	return request
}

func ptrString(value string) *string {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}

func ptrBool(value bool) *bool {
	return &value
}

func eventually(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("condition not met within timeout")
}

func roomsForDifferentSchedulerShards(shardCount int) (string, string) {
	for i := range 64 {
		hot := fmt.Sprintf("room-hot-%d", i)
		hotShard := schedulerTestShardIndex(hot, shardCount)

		for j := range 64 {
			cold := fmt.Sprintf("room-cold-%d", j)
			if hotShard != schedulerTestShardIndex(cold, shardCount) {
				return hot, cold
			}
		}
	}

	panic("failed to find rooms for different scheduler shards")
}

func schedulerTestShardIndex(key string, shardCount int) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	return int(hasher.Sum32() % uint32(shardCount))
}

func closeBlockingHandler(handler *Handler, release chan struct{}) {
	close(release)

	if err := handler.Close(); err != nil {
		panic(err)
	}
}

func TestServeHTTPQueueDepthObserved(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithWorkerCount(2),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t, t.Context(), validJSONBody())
	request.Header.Set(HeaderIrisToken, "token")
	handler.ServeHTTP(recorder, request)

	select {
	case <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}

	eventually(t, time.Second, func() bool {
		return metrics.accepted.Load() > 0
	})

	if metrics.queueDepthCalls.Load() == 0 {
		t.Fatal("ObserveQueueDepth was never called")
	}
}

func TestBuildMessageJSONIgnoresSenderRole(t *testing.T) {
	t.Parallel()

	var req WebhookRequest
	if err := jsonx.Unmarshal([]byte(`{"text":"hello","room":"room1","userId":"user1","senderRole":4}`), &req); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	msg := buildMessageJSON(req)

	out, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(out), "sender_role") {
		t.Fatalf("expected sender_role to be omitted, got %s", out)
	}
}

func TestBuildMessageJSONNilSenderRole(t *testing.T) {
	t.Parallel()

	req := WebhookRequest{
		Text:   "hello",
		Room:   "room1",
		UserID: "user1",
	}

	msg := buildMessageJSON(req)

	out, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(out), "sender_role") {
		t.Fatalf("expected sender_role to be omitted, got %s", out)
	}
}

func TestServeHTTPIgnoresSenderRole(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithWorkerCount(1),
		WithQueueSize(10),
	)
	defer closeHandler(t, handler)

	body := `{"text":"hi","room":"r1","userId":"u1","sender":"s1","senderRole":1}`
	request := newValidRequest(t, t.Context(), body)
	request.Header.Set(HeaderIrisToken, "token")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	// 워커가 메시지를 처리할 때까지 대기
	var received *Message
	select {
	case received = <-capture.msgCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive message")
	}

	if received.JSON == nil {
		t.Fatal("message JSON is nil")
	}
	out, err := jsonx.Marshal(received.JSON)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(out), "sender_role") {
		t.Fatalf("expected SenderRole to be ignored through full pipeline, got %s", out)
	}
}
