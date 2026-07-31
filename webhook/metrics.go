package webhook

import "time"

// Metrics는 webhook 핸들러 메트릭 관측 포인트를 정의합니다.
//
//nolint:interfacebloat // 이 인터페이스는 필요한 webhook 관측 포인트를 그대로 반영한다.
type Metrics interface {
	ObserveRequest()
	ObserveUnauthorized()
	ObserveBadRequest()
	ObserveDuplicate()
	ObserveEnqueueFailure()
	ObserveAccepted()
	ObserveDecodeLatency(d time.Duration)
	ObserveDedupLatency(d time.Duration)
	ObserveEnqueueWait(d time.Duration)
	ObserveQueueDepth(depth int)
	ObserveHandlerDuration(d time.Duration)
}

// DedupPendingObserver는 확정 전 예약 때문에 503으로 되돌린 요청을 Metrics 구현이 직접
// 계상하도록 선언하는 선택적 마커입니다. WithMetrics로 주입한 값이 이 메서드를 가지면
// Handler가 pending 거절마다 호출합니다. Metrics 인터페이스 자체는 바뀌지 않으므로 기존
// 구현은 그대로 컴파일되고, 구현하지 않아도 Handler.DedupPendingRejectedCount에서
// 누적값을 읽을 수 있습니다.
type DedupPendingObserver interface {
	ObserveDedupPendingRejected()
}

type NoopMetrics struct{}

func (NoopMetrics) ObserveRequest()                        {}
func (NoopMetrics) ObserveUnauthorized()                   {}
func (NoopMetrics) ObserveBadRequest()                     {}
func (NoopMetrics) ObserveDuplicate()                      {}
func (NoopMetrics) ObserveEnqueueFailure()                 {}
func (NoopMetrics) ObserveAccepted()                       {}
func (NoopMetrics) ObserveDecodeLatency(_ time.Duration)   {}
func (NoopMetrics) ObserveDedupLatency(_ time.Duration)    {}
func (NoopMetrics) ObserveEnqueueWait(_ time.Duration)     {}
func (NoopMetrics) ObserveQueueDepth(_ int)                {}
func (NoopMetrics) ObserveHandlerDuration(_ time.Duration) {}
