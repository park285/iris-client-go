package webhook

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// irisSenderAttemptTimeout은 Iris webhook worker의 attempt 상한(iris-runtime
// runtime/env/worker_profile/defaults.rs의 request_timeout_ms)이다. 이 저장소 코드만으로는 유도할 수 없다.
const irisSenderAttemptTimeout = 125 * time.Second

type mockMetrics struct {
	requests             atomic.Int32
	unauthorized         atomic.Int32
	badRequest           atomic.Int32
	duplicate            atomic.Int32
	enqueueFailure       atomic.Int32
	accepted             atomic.Int32
	decodeLatency        atomic.Int64
	handlerDuration      atomic.Int64
	handlerDurationCalls atomic.Int32
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

func signHandlerTestRequest(t *testing.T, request *http.Request, secret, body string) {
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

func (m *mockMetrics) ObserveHandlerDuration(d time.Duration) {
	m.handlerDuration.Add(int64(d))
	m.handlerDurationCalls.Add(1)
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n, err := b.buffer.Write(data)
	if err != nil {
		return n, fmt.Errorf("write locked buffer: %w", err)
	}

	return n, nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

type serveHTTPValidationCase struct {
	name        string
	method      string
	protoMajor  int
	headerToken string
	contentType string
	body        string
	opts        []HandlerOption
	wantStatus  int
	wantMetrics metricCounts
}

// recordingAdmitter는 순차 요청 테스트용이다. 동시 요청에는 acceptAdmitter를 쓴다.
type recordingAdmitter struct {
	calls int
	msg   *Message
	body  []byte
	err   error
}

func (a *recordingAdmitter) AdmitMessage(_ context.Context, msg *Message, body []byte) error {
	a.calls++

	a.msg = msg
	a.body = body

	return a.err
}

// acceptAdmitter는 상태 없이 모든 메시지를 수락한다.
type acceptAdmitter struct{}

func (acceptAdmitter) AdmitMessage(context.Context, *Message, []byte) error { return nil }

type closeAwareAdmitter struct {
	started chan struct{}
	done    chan error
}

func (a *closeAwareAdmitter) AdmitMessage(ctx context.Context, _ *Message, _ []byte) error {
	close(a.started)
	<-ctx.Done()

	a.done <- ctx.Err()

	return ctx.Err()
}

type blockingAdmitter struct {
	started chan struct{}
	release chan struct{}
}

func (a *blockingAdmitter) AdmitMessage(ctx context.Context, _ *Message, _ []byte) error {
	close(a.started)

	select {
	case <-a.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestServeHTTPDurableAdmissionTimeoutReturnsServiceUnavailable(t *testing.T) {
	admitTimeout := 50 * time.Millisecond
	admitter := &blockingAdmitter{started: make(chan struct{}), release: make(chan struct{})}
	handler := newTestHandler(t, admitter, WithAdmitTimeout(admitTimeout))

	defer closeHandler(t, handler)
	defer close(admitter.release)

	recorder := httptest.NewRecorder()
	startedAt := time.Now()
	done := beginServeHTTP(handler, recorder, acceptedCaseRequest(t))

	select {
	case <-admitter.started:
	case <-time.After(time.Second):
		t.Fatal("durable admission did not start")
	}

	select {
	case <-done:
	case <-time.After(admitTimeout + 500*time.Millisecond):
		t.Fatal("ServeHTTP did not return within the admission timeout bound")
	}

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if elapsed := time.Since(startedAt); elapsed < admitTimeout {
		t.Fatalf("ServeHTTP returned after %v, want at least %v", elapsed, admitTimeout)
	}
}

func TestNormalizeHandlerOptionsBoundsAdmitTimeoutByDefault(t *testing.T) {
	t.Parallel()

	if got := normalizeHandlerOptions(handlerOptions{}).admitTimeout; got != defaultAdmitTimeout {
		t.Fatalf("admitTimeout = %v, want the %v default", got, defaultAdmitTimeout)
	}

	if got := normalizeHandlerOptions(handlerOptions{admitTimeout: -time.Second}).admitTimeout; got != defaultAdmitTimeout {
		t.Fatalf("admitTimeout = %v for a non-positive request, want the %v default", got, defaultAdmitTimeout)
	}

	if defaultAdmitTimeout <= 0 || defaultAdmitTimeout >= irisSenderAttemptTimeout {
		t.Fatalf(
			"defaultAdmitTimeout = %v, must be positive and answer 503 before the sender abandons the attempt itself (%v)",
			defaultAdmitTimeout,
			irisSenderAttemptTimeout,
		)
	}
}

func TestServeHTTPDurableAdmissionTimeoutReleasesTheAdmissionGoroutine(t *testing.T) {
	admitter := &blockingAdmitter{started: make(chan struct{}), release: make(chan struct{})}
	handler := newTestHandler(t, admitter, WithAdmitTimeout(50*time.Millisecond))

	defer close(admitter.release)

	recorder := httptest.NewRecorder()
	done := beginServeHTTP(handler, recorder, acceptedCaseRequest(t))

	select {
	case <-admitter.started:
	case <-time.After(time.Second):
		t.Fatal("durable admission did not start")
	}

	assertServeHTTPCompletes(t, done, time.Second)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	closeCtx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	if err := handler.CloseContext(closeCtx); err != nil {
		t.Fatalf("CloseContext() error = %v, want the admission goroutine already released by the timeout", err)
	}
}

func TestServeHTTPDurableAdmissionCancelsBeforeTheDefaultAdmitTimeout(t *testing.T) {
	admitter := &blockingAdmitter{started: make(chan struct{}), release: make(chan struct{})}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)
	defer close(admitter.release)

	if got := handler.options.admitTimeout; got != defaultAdmitTimeout {
		t.Fatalf("admitTimeout = %v, want the %v default so admission is never unbounded", got, defaultAdmitTimeout)
	}

	requestCtx, cancelRequest := context.WithCancel(t.Context())
	defer cancelRequest()

	request := acceptedCaseRequest(t).WithContext(requestCtx)
	recorder := httptest.NewRecorder()
	done := beginServeHTTP(handler, recorder, request)

	select {
	case <-admitter.started:
	case <-time.After(time.Second):
		t.Fatal("durable admission did not start")
	}

	select {
	case <-done:
		t.Fatal("ServeHTTP returned before request cancellation")
	case <-time.After(100 * time.Millisecond):
	}

	cancelRequest()
	assertServeHTTPCompletes(t, done, time.Second)
	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
}

func TestDurableAdmissionPreservesCanonicalMessageID(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	request := newSignedIdentityRequest(t, validJSONBodyWithMessageID("header-message-id"), "header-message-id")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)

	if admitter.msg == nil || admitter.msg.JSON == nil || admitter.msg.JSON.MessageID != "header-message-id" {
		t.Fatalf("admitted message = %#v, want authenticated header identity", admitter.msg)
	}
}

func TestDurableAdmissionCloseContextCancelsCommitAfterGrace(t *testing.T) {
	t.Parallel()

	admitter := &closeAwareAdmitter{started: make(chan struct{}), done: make(chan error, 1)}
	handler := newTestHandler(t, admitter)
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

	closeCtx, cancel := context.WithCancel(t.Context())
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

func TestHandlerCloseContextReturnsNilAfterDrainWithCanceledContext(t *testing.T) {
	handler := newTestHandler(t, &recordingAdmitter{})
	if err := handler.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for range 1000 {
		if err := handler.CloseContext(ctx); err != nil {
			t.Fatalf("CloseContext() after drain error = %v, want nil", err)
		}
	}
}

func TestHandlerCloseContextPrefersDrainCompletedDuringCancellation(t *testing.T) {
	handler := &Handler{closeDone: make(chan struct{})}
	handler.closeOnce.Do(func() {})

	handler.runCancel = func() {
		close(handler.closeDone)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := handler.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext() error = %v, want nil after drain completed", err)
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

func TestServeHTTPAcceptedBuildsMessage(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{}

	handler := newAcceptedCaseHandler(t, metrics, admitter)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := acceptedCaseRequest(t)
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)
	assertAcceptedMessage(t, admitter)
	assertMetricCounts(t, metrics, metricCounts{requests: 1, accepted: 1})
}

func TestServeHTTPUnsupportedMediaTypeSkipsAdmission(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := validJSONBody()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set(HeaderIrisMessageID, "mid-unsupported")
	signHandlerTestRequest(t, request, testToken, body)

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusUnsupportedMediaType)

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0", admitter.calls)
	}
}

func TestServeHTTPMalformedBodySkipsAdmission(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter, WithMetrics(metrics))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newSignedIdentityRequest(t, "{invalid-json", "mid-early")

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusBadRequest)

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 for rejected body", admitter.calls)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, badRequest: 1})
}

func TestServeHTTPAdmissionFailureAfterClose(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter, WithMetrics(metrics))
	closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t.Context(), t, validJSONBody())
	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 after Close", admitter.calls)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, enqueueFailure: 1})
}

func TestServeHTTPLatencyMetricsRecorded(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	handler := newTestHandler(t, &recordingAdmitter{}, WithMetrics(metrics))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := newValidRequest(t.Context(), t, validJSONBodyWithMessageID("mid-lat"))

	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)

	if metrics.decodeLatency.Load() <= 0 {
		t.Fatal("decode latency not recorded")
	}

	if metrics.handlerDurationCalls.Load() != 1 {
		t.Fatalf("handler duration observations = %d, want 1", metrics.handlerDurationCalls.Load())
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
			name:        "unauthorized",
			method:      http.MethodPost,
			protoMajor:  1,
			headerToken: "wrong",
			contentType: contentTypeJSON,
			body:        validJSONBody(),
			wantStatus:  http.StatusUnauthorized,
			wantMetrics: metricCounts{requests: 1, unauthorized: 1},
		},
		{
			name:        "missing content type",
			method:      http.MethodPost,
			protoMajor:  1,
			headerToken: testToken,
			body:        validJSONBody(),
			wantStatus:  http.StatusUnsupportedMediaType,
			wantMetrics: metricCounts{requests: 1},
		},
		{
			name:        "invalid content type",
			method:      http.MethodPost,
			protoMajor:  1,
			headerToken: testToken,
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
		headerToken: testToken,
		contentType: contentTypeJSON,
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
			headerToken: testToken,
			contentType: contentTypeJSON,
			body:        `{"text":"","room":"room-1","userId":"user-1"}`,
			wantStatus:  http.StatusBadRequest,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
		{
			name:        "body too large",
			method:      http.MethodPost,
			protoMajor:  1,
			headerToken: testToken,
			contentType: contentTypeJSON,
			body:        validJSONBody(),
			opts:        []HandlerOption{WithMaxBodyBytes(8)},
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
		{
			name:        "attachment too large",
			method:      http.MethodPost,
			protoMajor:  1,
			headerToken: testToken,
			contentType: contentTypeJSON,
			body:        `{"text":"hello","room":"room-1","userId":"user-1","attachment":"` + strings.Repeat("x", 65537) + `"}`,
			wantStatus:  http.StatusBadRequest,
			wantMetrics: metricCounts{requests: 1, badRequest: 1},
		},
	}
}

func runServeHTTPValidationCase(t *testing.T, tt serveHTTPValidationCase) {
	t.Helper()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{}

	handler := newTestHandler(t, admitter, append([]HandlerOption{WithMetrics(metrics)}, tt.opts...)...)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), tt.method, "/webhook/iris", strings.NewReader(tt.body))
	setRequestProtoMajor(request, tt.protoMajor)
	setRequestHeader(request, "Content-Type", tt.contentType)

	if tt.headerToken == testToken {
		signHandlerTestRequest(t, request, tt.headerToken, tt.body)
	}

	handler.ServeHTTP(recorder, request)
	assertResponseCode(t, recorder.Code, tt.wantStatus)
	assertMetricCounts(t, metrics, tt.wantMetrics)

	if admitter.calls != 0 {
		t.Fatalf("admission calls = %d, want 0 for a rejected request", admitter.calls)
	}
}

func newAcceptedCaseHandler(t *testing.T, metrics *mockMetrics, admitter MessageAdmitter) *Handler {
	t.Helper()

	return newTestHandler(t, admitter, WithWebhookToken(" token "), WithMetrics(metrics))
}

func acceptedCaseRequest(t *testing.T) *http.Request {
	t.Helper()

	body := `{"route":" default ","messageId":" msg-1 ","sourceLogId":1000000000001,"sourceCreatedAtMs":1700000000000,"rawSourceLogId":1,"sourceGenerationId":1,"sourceAccountId":" 123456789 ","text":" hello ","room":" room-1 ","sender":" tester ","userId":" user-1 ","chatLogId":" chat-1 ","roomType":" OD ","roomLinkId":" room-link ","threadId":" 123 ","threadScope":2,"type":" 1 ","isMine":true,"origin":" WRITE ","attachment":"{\"url\":\"test\"}"}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set(HeaderIrisMessageID, " msg-1 ")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	signHandlerTestRequest(t, request, testToken, body)

	return request
}

func assertAcceptedResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	assertResponseCode(t, recorder.Code, http.StatusOK)
}

func assertAcceptedMessage(t *testing.T, admitter *recordingAdmitter) {
	t.Helper()

	threadScope := 2
	want := &Message{
		Msg:    " hello ",
		Room:   " room-1 ",
		Sender: new("tester"),
		JSON: &MessageJSON{
			UserID:             " user-1 ",
			Type:               "1",
			Route:              "default",
			MessageID:          "msg-1",
			ChatLogID:          "chat-1",
			RoomType:           "OD",
			RoomLinkID:         "room-link",
			SourceLogID:        new(int64(1_000_000_000_001)),
			SourceCreatedAtMS:  1_700_000_000_000,
			RawSourceLogID:     new(int64(1)),
			SourceGenerationID: new(int64(1)),
			SourceAccountID:    "123456789",
			ThreadID:           new("123"),
			ThreadScope:        &threadScope,
			IsMine:             new(true),
			Origin:             "WRITE",
			Attachment:         "{\"url\":\"test\"}",
		},
	}

	if admitter.calls != 1 || !reflect.DeepEqual(admitter.msg, want) {
		t.Fatalf("admission = calls:%d msg:%#v, want one %#v", admitter.calls, admitter.msg, want)
	}
}

func TestServeHTTPAcceptedPreservesEventPayload(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newAcceptedCaseHandler(t, &mockMetrics{}, admitter)

	defer closeHandler(t, handler)

	body := `{"messageId":"msg-event-1","text":"{\"type\":\"member_nickname_updated\"}","room":"room-a","sender":"iris-system","userId":"0","type":"member_nickname_updated","eventPayload":{"previousDisplayName":"alice","currentDisplayName":"alice2","createdAtMs":1778226335000}}`
	request := newValidRequest(t.Context(), t, body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)

	got := admitter.msg
	if got == nil || got.JSON == nil {
		t.Fatalf("message = %#v, want payload-bearing message", got)
	}

	if got.JSON.MessageID != "msg-event-1" || got.JSON.Type != testEventTypeMemberNicknameUpdated {
		t.Fatalf("MessageID/Type = %q/%q, want msg-event-1/%q", got.JSON.MessageID, got.JSON.Type, testEventTypeMemberNicknameUpdated)
	}

	assertNicknamePayload(t, got.JSON.EventPayload)
}

func TestServeHTTPAcceptedPreservesEventPayloadWithoutText(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newAcceptedCaseHandler(t, &mockMetrics{}, admitter)

	defer closeHandler(t, handler)

	body := `{"messageId":"msg-event-no-text-1","room":"room-a","sender":"iris-system","userId":"0","type":"member_nickname_updated","eventPayload":{"previousDisplayName":"alice","currentDisplayName":"alice2","createdAtMs":1778226335000}}`
	request := newValidRequest(t.Context(), t, body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)

	got := admitter.msg
	if got == nil || got.JSON == nil {
		t.Fatalf("message = %#v, want payload-bearing message", got)
	}

	if got.Msg != "" {
		t.Fatalf("Msg = %q, want empty event marker text", got.Msg)
	}

	if got.JSON.Type != testEventTypeMemberNicknameUpdated {
		t.Fatalf("Type = %q, want %q", got.JSON.Type, testEventTypeMemberNicknameUpdated)
	}

	assertNicknamePayload(t, got.JSON.EventPayload)
}

func assertNicknamePayload(t *testing.T, raw []byte) {
	t.Helper()

	var payload map[string]any

	if err := jsonv2.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal(EventPayload) error = %v", err)
	}

	if payload["previousDisplayName"] != testSenderAlice || payload["currentDisplayName"] != "alice2" {
		t.Fatalf("EventPayload = %#v, want nickname payload", payload)
	}
}

func TestServeHTTPAcceptedPreservesTypeOnlyObserverPayload(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newAcceptedCaseHandler(t, &mockMetrics{}, admitter)

	defer closeHandler(t, handler)

	body := `{"route":"chatbotgo-observer","messageId":"msg-observer-type-only-1","text":"","room":"room-a","sender":"iris-system","userId":"user-1","type":"0","origin":"MSG"}`
	request := newValidRequest(t.Context(), t, body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertAcceptedResponse(t, recorder)

	got := admitter.msg
	if got == nil || got.JSON == nil {
		t.Fatalf("message = %#v, want type-only observer message", got)
	}

	if got.Msg != "" || got.JSON.Route != "chatbotgo-observer" || got.JSON.Type != "0" {
		t.Fatalf("message = %#v, want preserved observer route and type", got)
	}
}

func TestBuildMessageJSON_DoesNotFallbackThreadIDFromChatLogID(t *testing.T) {
	got := buildMessageJSON(WebhookRequest{
		Text:       testHelloText,
		Room:       testRoom1,
		UserID:     testUserID1,
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
		Room:         testRoom1,
		UserID:       "0",
		Type:         testEventTypeMemberNicknameUpdated,
		EventPayload: []byte(`{"previousDisplayName":"alice","currentDisplayName":"alice2"}`),
	})

	if string(got.EventPayload) != `{"previousDisplayName":"alice","currentDisplayName":"alice2"}` {
		t.Fatalf("EventPayload = %s, want raw payload", got.EventPayload)
	}
}

func TestBuildMessageJSONCopiesMentions(t *testing.T) {
	got := buildMessageJSON(WebhookRequest{
		Text:   "!누구 @카푸치노",
		Room:   testRoomA,
		UserID: testUserID1,
		Mentions: []WebhookMention{
			{UserID: "8691114094424718810", At: []int{4}, Len: 4},
		},
	})

	want := []WebhookMention{{UserID: "8691114094424718810", At: []int{4}, Len: 4}}
	if !reflect.DeepEqual(got.Mentions, want) {
		t.Fatalf("MessageJSON.Mentions = %#v, want %#v", got.Mentions, want)
	}
}

func beginServeHTTP(handler *Handler, recorder *httptest.ResponseRecorder, request *http.Request) <-chan struct{} {
	done := make(chan struct{}, 1)

	go func() {
		handler.ServeHTTP(recorder, request)

		done <- struct{}{}
	}()

	return done
}

func assertServeHTTPCompletes(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("ServeHTTP did not return")
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
	return validJSONBodyWithMessageID("webhook-test-message-id")
}

func validJSONBodyWithMessageID(messageID string) string {
	return fmt.Sprintf(`{"messageId":%q,"text":"hello","room":"room-1","sender":"tester","userId":"user-1"}`, messageID)
}

func newValidRequest(ctx context.Context, t *testing.T, body string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/webhook/iris", strings.NewReader(body))
	request.Header.Set("Content-Type", contentTypeJSON)
	signHandlerTestRequest(t, request, testToken, body)

	return request
}

func TestBuildMessageJSONIgnoresSenderRole(t *testing.T) {
	t.Parallel()

	var req WebhookRequest

	if err := jsonv2.Unmarshal([]byte(`{"text":"hello","room":"room1","userId":"user1","senderRole":4}`), &req); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	msg := buildMessageJSON(req)

	out, err := jsonv2.Marshal(msg)
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
		Text:   testHelloText,
		Room:   "room1",
		UserID: "user1",
	}

	msg := buildMessageJSON(req)

	out, err := jsonv2.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if strings.Contains(string(out), "sender_role") {
		t.Fatalf("expected sender_role to be omitted, got %s", out)
	}
}

func TestServeHTTPIgnoresSenderRole(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := newTestHandler(t, admitter)

	defer closeHandler(t, handler)

	body := `{"messageId":"webhook-test-message-id","text":"hi","room":"r1","userId":"u1","sender":"s1","senderRole":1}`
	request := newValidRequest(t.Context(), t, body)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertResponseCode(t, recorder.Code, http.StatusOK)

	received := admitter.msg
	if received == nil || received.JSON == nil {
		t.Fatalf("admitted message = %#v, want message JSON", received)
	}

	out, err := jsonv2.Marshal(received.JSON)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if strings.Contains(string(out), "sender_role") {
		t.Fatalf("expected SenderRole to be ignored through full pipeline, got %s", out)
	}
}

// validation 경로가 nonce timeout을 무조건 deadline으로 쓰므로, 0이나 음수가 그대로 남으면 모든
// nonce 왕복이 즉시 만료된다.
func TestNonceTimeoutIsAlwaysNormalizedBeforeUse(t *testing.T) {
	t.Parallel()

	for _, requested := range []time.Duration{0, -time.Second} {
		if got := normalizeHandlerOptions(handlerOptions{nonceTimeout: requested}).nonceTimeout; got != defaultNonceTimeout {
			t.Fatalf("normalizeHandlerOptions(nonceTimeout=%v) = %v, want %v", requested, got, defaultNonceTimeout)
		}
	}

	handler := newTestHandler(t, &recordingAdmitter{}, WithNonceTimeout(0))

	defer closeHandler(t, handler)

	if handler.options.nonceTimeout != defaultNonceTimeout {
		t.Fatalf("WithNonceTimeout(0) left nonceTimeout = %v, want %v", handler.options.nonceTimeout, defaultNonceTimeout)
	}
}
