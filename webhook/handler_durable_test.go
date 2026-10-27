package webhook

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDurableHandlerCommitsBeforeOK(t *testing.T) {
	t.Parallel()

	admitter := &recordingAdmitter{}
	handler := mustNewDurableHandler(t, admitter, slog.Default())

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusOK)

	if admitter.calls != 1 || admitter.msg == nil {
		t.Fatalf("admission = calls:%d msg:%#v, want one committed message", admitter.calls, admitter.msg)
	}
}

func TestNewDurableHandlerAdmissionFailureReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{err: errors.New("commit failed")}
	handler := mustNewDurableHandler(t, admitter, slog.Default(), WithMetrics(metrics))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, enqueueFailure: 1})
}

func TestNewDurableHandlerAlreadyAdmittedReturnsOKAndObservesDuplicate(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	admitter := &recordingAdmitter{err: fmt.Errorf("inbox insert: %w", ErrAlreadyAdmitted)}
	handler := mustNewDurableHandler(t, admitter, slog.Default(), WithMetrics(metrics))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusOK)

	if admitter.calls != 1 {
		t.Fatalf("admission calls = %d, want 1", admitter.calls)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, duplicate: 1})
}

func TestNewDurableHandlerObservesHandlerDuration(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	handler := mustNewDurableHandler(t, &recordingAdmitter{}, slog.Default(), WithMetrics(metrics))

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
	handler := mustNewDurableHandler(t, admitter, slog.Default(), WithMetrics(metrics))

	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, acceptedCaseRequest(t))

	assertResponseCode(t, recorder.Code, http.StatusServiceUnavailable)

	if calls := metrics.handlerDurationCalls.Load(); calls != 1 {
		t.Fatalf("handler duration observations = %d, want 1 on durable admit failure", calls)
	}
}
