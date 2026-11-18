package transport

import "time"

type TransportMetrics interface {
	ObserveReplyRetry(attempt int, delay time.Duration)
	ObserveReplyRetryAfter(delay time.Duration)
}

type NoopTransportMetrics struct{}

func (NoopTransportMetrics) ObserveReplyRetry(_ int, _ time.Duration) {}
func (NoopTransportMetrics) ObserveReplyRetryAfter(_ time.Duration)   {}
