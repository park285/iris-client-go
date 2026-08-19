package webhook

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDurableDiagnosticsSchedulerDisabled(t *testing.T) {
	handler := mustNewDurableHandler(t, &recordingAdmitter{}, slog.Default(),
		WithNonceStore(newMemoryNonceCache()),
		WithWorkerCount(99),
		WithQueueSize(999),
	)
	t.Cleanup(func() { closeHandler(t, handler) })

	diagnostics := handler.Diagnostics()
	if diagnostics.SchedulerEnabled || diagnostics.WorkersConfigured != 0 || diagnostics.QueueSize != 0 ||
		diagnostics.Pending != 0 || diagnostics.InFlight != 0 {
		t.Fatalf("Diagnostics() = %+v, want scheduler disabled without synthetic capacity", diagnostics)
	}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"workersConfigured", "queueSize", "pending", "inFlight"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("Diagnostics JSON = %s, unexpectedly contains %q", encoded, forbidden)
		}
	}
}

const durableAdmissionWarnFragment = "not invoked in durable admission mode"

func mustNewDurableHandler(t *testing.T, admitter MessageAdmitter, logger *slog.Logger, opts ...HandlerOption) *Handler {
	t.Helper()

	handler, err := NewDurableHandler(t.Context(), "token", admitter, logger, opts...)
	if err != nil {
		t.Fatalf("NewDurableHandler() error = %v", err)
	}

	return handler
}

func TestNewDurableHandlerCommitsBeforeOKWithoutMessageHandler(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	dedup := &mockDeduplicator{duplicate: true}
	handler := mustNewDurableHandler(t, admitter, slog.Default(),
		WithMessageDeduplicator(dedup),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

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
	if handler.handler != nil {
		t.Fatalf("message handler = %T, want nil in durable-only mode", handler.handler)
	}
}

func TestNewDurableHandlerRejectsNilAdmitter(t *testing.T) {
	t.Parallel()

	handler, err := NewDurableHandler(t.Context(), "token", nil, slog.Default())
	if !errors.Is(err, ErrMessageAdmitterRequired) {
		t.Fatalf("NewDurableHandler(nil admitter) error = %v, want %v", err, ErrMessageAdmitterRequired)
	}
	if handler != nil {
		t.Fatalf("handler = %#v, want nil on nil admitter", handler)
	}
}

func TestNewDurableHandlerPositionalAdmitterOverridesOptionAdmitter(t *testing.T) {
	t.Parallel()

	positional := &recordingAdmitter{}
	optional := &recordingAdmitter{}
	handler := mustNewDurableHandler(t, positional, slog.Default(),
		WithDurableAdmission(optional),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusOK)
	if positional.calls != 1 || optional.calls != 0 {
		t.Fatalf("admitter calls positional/option = %d/%d, want 1/0", positional.calls, optional.calls)
	}
}

func TestNewDurableHandlerAdmissionFailureReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{err: errors.New("commit failed")}
	handler := mustNewDurableHandler(t, admitter, slog.Default(), WithNonceStore(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}
}

func TestNewDurableHandlerObservesHandlerDuration(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	handler := mustNewDurableHandler(t, &recordingAdmitter{}, slog.Default(),
		WithMetrics(metrics),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusOK)
	if calls := metrics.handlerDurationCalls.Load(); calls != 1 {
		t.Fatalf("handler duration observations = %d, want 1 on durable admit success", calls)
	}
}

func TestNewDurableHandlerObservesHandlerDurationOnAdmissionFailure(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{err: errors.New("commit failed")}
	handler := mustNewDurableHandler(t, admitter, slog.Default(),
		WithMetrics(metrics),
		WithNonceStore(newMemoryNonceCache()),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)
	if calls := metrics.handlerDurationCalls.Load(); calls != 1 {
		t.Fatalf("handler duration observations = %d, want 1 on durable admit failure", calls)
	}
}

func TestNewDurableHandlerDoesNotWarnAboutIgnoredMessageHandler(t *testing.T) {
	t.Parallel()

	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := mustNewDurableHandler(t, &recordingAdmitter{}, logger, WithNonceStore(newMemoryNonceCache()))
	closeHandler(t, handler)

	if strings.Contains(logs.String(), durableAdmissionWarnFragment) {
		t.Fatalf("unexpected durable admission warning: %s", logs.String())
	}
}

func TestNewHandlerWarnsWhenMessageHandlerCombinedWithDurableAdmission(t *testing.T) {
	t.Parallel()

	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := newTestHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, logger,
		WithDurableAdmission(&recordingAdmitter{}),
		WithNonceStore(newMemoryNonceCache()),
	)
	closeHandler(t, handler)

	if !strings.Contains(logs.String(), durableAdmissionWarnFragment) {
		t.Fatalf("missing durable admission warning, logs: %s", logs.String())
	}
}
