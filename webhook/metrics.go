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
