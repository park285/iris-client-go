package transport

import "time"

// 콜백은 재시도·재연결 경로에서 동기 호출되므로 구현은 동시 호출을 견디고 즉시 반환해야 한다.
type TransportMetrics interface {
	ObserveReplyRetry(attempt int, delay time.Duration)
	ObserveReplyRetryAfter(delay time.Duration)
	ObserveSSEReconnectAttempt(attempt int)
	ObserveSSEReconnectFailure(attempt int)
	ObserveSSEReconnectSuccess(attempt int)
}

type NoopTransportMetrics struct{}

func (NoopTransportMetrics) ObserveReplyRetry(_ int, _ time.Duration) {}
func (NoopTransportMetrics) ObserveReplyRetryAfter(_ time.Duration)   {}
func (NoopTransportMetrics) ObserveSSEReconnectAttempt(_ int)         {}
func (NoopTransportMetrics) ObserveSSEReconnectFailure(_ int)         {}
func (NoopTransportMetrics) ObserveSSEReconnectSuccess(_ int)         {}
