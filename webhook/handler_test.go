package webhook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	iris "park285/iris-client-go"
)

type mockMetrics struct {
	requests       atomic.Int32
	unauthorized   atomic.Int32
	badRequest     atomic.Int32
	duplicate      atomic.Int32
	enqueueFailure atomic.Int32
	accepted       atomic.Int32
}

type metricCounts struct {
	requests       int32
	unauthorized   int32
	badRequest     int32
	duplicate      int32
	enqueueFailure int32
	accepted       int32
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

type captureHandler struct {
	msgCh chan *iris.Message
}

func (h *captureHandler) HandleMessage(_ context.Context, msg *iris.Message) {
	select {
	case h.msgCh <- msg:
	default:
	}
}

type blockingHandler struct {
	started chan struct{}
	block   chan struct{}
}

func (h *blockingHandler) HandleMessage(_ context.Context, _ *iris.Message) {
	select {
	case h.started <- struct{}{}:
	default:
	}

	<-h.block
}

type countingBlockingHandler struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (h *countingBlockingHandler) HandleMessage(_ context.Context, _ *iris.Message) {
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

func (h *panicHandler) HandleMessage(_ context.Context, _ *iris.Message) {
	h.calls.Add(1)
	panic("boom")
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
	capture := &captureHandler{msgCh: make(chan *iris.Message, 1)}

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
	capture := &captureHandler{msgCh: make(chan *iris.Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t.Context(), validJSONBody())
	request.Header.Set(iris.HeaderIrisToken, "token")
	request.Header.Set(iris.HeaderIrisMessageID, "mid-1")

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

func TestServeHTTPDedupErrorDegradesToAccepted(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{err: errors.New("boom")}
	capture := &captureHandler{msgCh: make(chan *iris.Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t.Context(), validJSONBody())
	request.Header.Set(iris.HeaderIrisToken, "token")
	request.Header.Set(iris.HeaderIrisMessageID, "mid-1")

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
		&captureHandler{msgCh: make(chan *iris.Message, 1)},
		slog.Default(),
		WithMetrics(metrics),
	)
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t.Context(), validJSONBody())
	request.Header.Set(iris.HeaderIrisToken, "token")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, enqueueFailure: 1})
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
		WithQueueSize(1),
		WithEnqueueTimeout(10*time.Millisecond),
	)
	defer closeBlockingHandler(handler, blocker.block)

	first := httptest.NewRecorder()
	req1 := newValidRequest(t.Context(), validJSONBody())
	req1.Header.Set(iris.HeaderIrisToken, "token")
	handler.ServeHTTP(first, req1)

	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusOK)
	}

	select {
	case <-blocker.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	second := httptest.NewRecorder()
	req2 := newValidRequest(t.Context(), validJSONBody())
	req2.Header.Set(iris.HeaderIrisToken, "token")
	handler.ServeHTTP(second, req2)

	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusOK)
	}

	third := httptest.NewRecorder()
	req3 := newValidRequest(t.Context(), validJSONBody())
	req3.Header.Set(iris.HeaderIrisToken, "token")
	handler.ServeHTTP(third, req3)

	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("third status = %d, want %d", third.Code, http.StatusServiceUnavailable)
	}
}

func TestStripeKey(t *testing.T) {
	t.Parallel()

	threadID := "thread-1"
	tests := []struct {
		name string
		msg  *iris.Message
		want string
	}{
		{name: "nil message", want: ""},
		{name: "room only", msg: &iris.Message{Room: " room-1 "}, want: "room-1"},
		{name: "room and thread", msg: &iris.Message{Room: "room-1", JSON: &iris.MessageJSON{ThreadID: &threadID}}, want: "room-1:thread-1"},
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

func TestStripeIndexUsesThreadPartition(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		t.Context(),
		"token",
		&captureHandler{msgCh: make(chan *iris.Message, 1)},
		slog.Default(),
		WithWorkerCount(8),
	)
	defer closeHandler(t, handler)

	threadA := "thread-a"
	threadB := "thread-b"
	msgA := &iris.Message{Room: "room-1", JSON: &iris.MessageJSON{ThreadID: &threadA}}
	msgB := &iris.Message{Room: "room-1", JSON: &iris.MessageJSON{ThreadID: &threadB}}

	if stripeKey(msgA) == stripeKey(msgB) {
		t.Fatal("test bug: expected different stripe keys")
	}

	foundDifferent := false

	for range 32 {
		if handler.stripeIndex(msgA) != handler.stripeIndex(msgB) {
			foundDifferent = true
			break
		}
	}

	if !foundDifferent {
		t.Fatal("expected different thread keys to map to different stripes at least once")
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

func TestWorkerRecoversFromPanic(t *testing.T) {
	t.Parallel()

	worker := &panicHandler{}

	handler := NewHandler(
		t.Context(),
		"token",
		worker,
		slog.Default(),
		WithWorkerCount(1),
	)
	defer closeHandler(t, handler)

	if err := handler.enqueue(webhookTask{msg: &iris.Message{Msg: "msg"}}); err != nil {
		t.Fatalf("enqueue error = %v", err)
	}

	eventually(t, time.Second, func() bool {
		return worker.calls.Load() == 1
	})
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
		{
			name:        "http2 required",
			method:      http.MethodPost,
			protoMajor:  1,
			token:       "token",
			headerToken: "token",
			contentType: "application/json",
			body:        validJSONBody(),
			opts:        []HandlerOption{WithRequireHTTP2(true)},
			wantStatus:  http.StatusHTTPVersionNotSupported,
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
	}
}

func runServeHTTPValidationCase(t *testing.T, tt serveHTTPValidationCase) {
	t.Helper()

	metrics := &mockMetrics{}

	handler := NewHandler(
		t.Context(),
		tt.token,
		&captureHandler{msgCh: make(chan *iris.Message, 1)},
		slog.Default(),
		append([]HandlerOption{WithMetrics(metrics)}, tt.opts...)...,
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), tt.method, "/webhook/iris", strings.NewReader(tt.body))
	setRequestProtoMajor(request, tt.protoMajor)
	setRequestHeader(request, iris.HeaderIrisToken, tt.headerToken)
	setRequestHeader(request, "Content-Type", tt.contentType)
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
		WithDedupTTL(90*time.Second),
	)
}

func acceptedCaseRequest(t *testing.T) *http.Request {
	t.Helper()

	body := `{"route":" default ","messageId":" msg-1 ","sourceLogId":42,"text":" hello ","room":" room-1 ","sender":" tester ","userId":" user-1 ","chatLogId":" chat-1 ","roomType":" OD ","roomLinkId":" room-link ","threadId":" 123 ","threadScope":2}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(iris.HeaderIrisToken, "token")
	request.Header.Set(iris.HeaderIrisMessageID, " msg-header ")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	return request
}

func assertAcceptedResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	assertResponseCode(t, recorder.Code, http.StatusOK)
}

func assertAcceptedMessage(t *testing.T, capture *captureHandler) {
	t.Helper()

	threadScope := 2
	want := &iris.Message{
		Msg:    " hello ",
		Room:   " room-1 ",
		Sender: ptrString("tester"),
		JSON: &iris.MessageJSON{
			UserID:      " user-1 ",
			Message:     " hello ",
			ChatID:      " room-1 ",
			Route:       "default",
			MessageID:   "msg-1",
			ChatLogID:   "chat-1",
			RoomType:    "OD",
			RoomLinkID:  "room-link",
			SourceLogID: ptrInt64(42),
			ThreadID:    ptrString("123"),
			ThreadScope: &threadScope,
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

func assertAcceptedDedup(t *testing.T, dedup *mockDeduplicator) {
	t.Helper()

	calls := dedup.snapshot()
	if len(calls) != 1 {
		t.Fatalf("dedup call count = %d, want %d", len(calls), 1)
	}

	if calls[0].key != "iris:msg:{msg-header}" {
		t.Fatalf("dedup key = %q, want %q", calls[0].key, "iris:msg:{msg-header}")
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
	task := webhookTask{msg: &iris.Message{Msg: "msg"}}
	mustEnqueue(t, handler, task, "first")
	mustEnqueue(t, handler, task, "second")

	return worker, handler, task
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

func newValidRequest(ctx context.Context, body string) *http.Request {
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	return request
}

func ptrString(value string) *string {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}

func eventually(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition not met before timeout")
}

func closeBlockingHandler(handler *Handler, release chan struct{}) {
	close(release)

	if err := handler.Close(); err != nil {
		panic(err)
	}
}
