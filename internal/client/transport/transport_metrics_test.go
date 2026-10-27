package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
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

	client := mustNewAPIClient(
		"http://iris.test",
		"",
		WithReplyRetry(2),
		WithTransportMetrics(metrics),
		withRoundTripper(transportMetricsRoundTripFunc(func(*http.Request) (*http.Response, error) {
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
		done <- sendMessage(ctx, client, testRoom, "message")
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
	client := mustNewAPIClient(
		"http://iris.test",
		"",
		WithReplyRetry(2),
		WithTransportMetrics(metrics),
		withRoundTripper(transportMetricsRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(http.NoBody),
			}, nil
		})),
	)

	err := sendMessage(t.Context(), client, testRoom, "message")
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
