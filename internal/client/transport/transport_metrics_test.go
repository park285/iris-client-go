package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type transportMetricEvent struct {
	name    string
	attempt int
	delay   time.Duration
}

type recordingTransportMetrics struct {
	events chan transportMetricEvent
}

func newRecordingTransportMetrics() *recordingTransportMetrics {
	return &recordingTransportMetrics{events: make(chan transportMetricEvent, 16)}
}

func (m *recordingTransportMetrics) ObserveReplyRetry(attempt int, delay time.Duration) {
	m.events <- transportMetricEvent{name: "reply_retry", attempt: attempt, delay: delay}
}

func (m *recordingTransportMetrics) ObserveReplyRetryAfter(delay time.Duration) {
	m.events <- transportMetricEvent{name: "reply_retry_after", delay: delay}
}

func (m *recordingTransportMetrics) ObserveSSEReconnectAttempt(attempt int) {
	m.events <- transportMetricEvent{name: "sse_reconnect_attempt", attempt: attempt}
}

func (m *recordingTransportMetrics) ObserveSSEReconnectFailure(attempt int) {
	m.events <- transportMetricEvent{name: "sse_reconnect_failure", attempt: attempt}
}

func (m *recordingTransportMetrics) ObserveSSEReconnectSuccess(attempt int) {
	m.events <- transportMetricEvent{name: "sse_reconnect_success", attempt: attempt}
}

func TestApplyClientOptionsUsesNoopTransportMetricsByDefault(t *testing.T) {
	got := applyClientOptions(nil)
	if _, ok := got.TransportMetrics.(NoopTransportMetrics); !ok {
		t.Fatalf("TransportMetrics = %T, want NoopTransportMetrics", got.TransportMetrics)
	}
}

func TestWithTransportMetricsAppliesObserver(t *testing.T) {
	metrics := newRecordingTransportMetrics()
	got := applyClientOptions([]ClientOption{WithTransportMetrics(metrics)})
	if got.TransportMetrics != metrics {
		t.Fatalf("TransportMetrics = %T, want recording observer", got.TransportMetrics)
	}
}

func TestTransportMetricsObserveReplyRetryAndRetryAfter(t *testing.T) {
	metrics := newRecordingTransportMetrics()
	var requests atomic.Int32
	client := NewH2CClient(
		"http://iris.test",
		"",
		WithReplyRetry(2),
		WithTransportMetrics(metrics),
		WithRoundTripper(transportMetricsRoundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"1"}},
				Body:       io.NopCloser(http.NoBody),
			}, nil
		})),
	)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- client.SendMessage(ctx, "room", "message")
	}()

	assertTransportMetricEvent(t, metrics.events, transportMetricEvent{
		name:    "reply_retry",
		attempt: 1,
		delay:   time.Second,
	})
	assertTransportMetricEvent(t, metrics.events, transportMetricEvent{
		name:  "reply_retry_after",
		delay: time.Second,
	})
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SendMessage() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendMessage() did not stop after cancellation")
	}

	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestTransportMetricsDoesNotObserveRetryAfterWithoutHeader(t *testing.T) {
	metrics := newRecordingTransportMetrics()
	client := NewH2CClient(
		"http://iris.test",
		"",
		WithReplyRetry(2),
		WithTransportMetrics(metrics),
		WithRoundTripper(transportMetricsRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(http.NoBody),
			}, nil
		})),
	)

	err := client.SendMessage(t.Context(), "room", "message")
	if err == nil {
		t.Fatal("SendMessage() error = nil, want rate limit error")
	}

	select {
	case got := <-metrics.events:
		if got.name != "reply_retry" || got.attempt != 1 {
			t.Fatalf("metric event = %+v, want reply_retry attempt 1", got)
		}
	default:
		t.Fatal("reply retry metric was not observed")
	}
	select {
	case got := <-metrics.events:
		t.Fatalf("unexpected metric event = %+v", got)
	default:
	}
}

func TestTransportMetricsSSEReconnectCancellationOnlyObservesAttempt(t *testing.T) {
	metrics := newRecordingTransportMetrics()
	reconnectStarted := make(chan struct{})
	var requests atomic.Int32
	client := NewH2CClient(
		"http://iris.test",
		"",
		WithTransportMetrics(metrics),
		WithRoundTripper(transportMetricsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(http.NoBody),
					Request:    req,
				}, nil
			}
			close(reconnectStarted)
			<-req.Context().Done()
			return nil, req.Context().Err()
		})),
	)

	ctx, cancel := context.WithCancel(t.Context())
	stream, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		cancel()
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	select {
	case <-reconnectStarted:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("reconnect request did not start")
	}
	assertTransportMetricEvent(t, metrics.events, transportMetricEvent{
		name:    "sse_reconnect_attempt",
		attempt: 1,
	})

	cancel()
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("event stream emitted an unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("event stream did not close after cancellation")
	}
	select {
	case got := <-metrics.events:
		t.Fatalf("unexpected metric event after cancellation = %+v", got)
	default:
	}
}

func TestTransportMetricsObserveSSEReconnectLifecycle(t *testing.T) {
	metrics := newRecordingTransportMetrics()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case 2:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case 3:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	client := NewH2CClient(
		server.URL,
		"",
		WithTransport("http1"),
		WithTransportMetrics(metrics),
	)
	stream, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		cancel()
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	for _, want := range []transportMetricEvent{
		{name: "sse_reconnect_attempt", attempt: 1},
		{name: "sse_reconnect_failure", attempt: 1},
		{name: "sse_reconnect_attempt", attempt: 2},
		{name: "sse_reconnect_success", attempt: 2},
	} {
		assertTransportMetricEvent(t, metrics.events, want)
	}

	cancel()
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("event stream emitted an unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("event stream did not close after cancellation")
	}
}

func assertTransportMetricEvent(t *testing.T, events <-chan transportMetricEvent, want transportMetricEvent) {
	t.Helper()

	select {
	case got := <-events:
		if got != want {
			t.Fatalf("metric event = %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for metric event %+v", want)
	}
}

type transportMetricsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f transportMetricsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
