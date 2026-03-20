package webhook

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
}

// NoopMetrics is the default no-op implementation of Metrics.
type NoopMetrics struct{}

func (NoopMetrics) ObserveRequest()        {}
func (NoopMetrics) ObserveUnauthorized()   {}
func (NoopMetrics) ObserveBadRequest()     {}
func (NoopMetrics) ObserveDuplicate()      {}
func (NoopMetrics) ObserveEnqueueFailure() {}
func (NoopMetrics) ObserveAccepted()       {}
