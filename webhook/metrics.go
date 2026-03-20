package webhook

import "time"

// Metrics defines webhook handler metric observation points.
//
//nolint:interfacebloat // The interface mirrors the required webhook observation points.
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

// NoopMetrics is the default no-op implementation of Metrics.
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
