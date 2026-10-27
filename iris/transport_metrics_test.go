package iris_test

import (
	"time"

	"github.com/park285/iris-client-go/v3/iris"
)

type noopTransportMetrics struct{}

func (noopTransportMetrics) ObserveReplyRetry(int, time.Duration) {}
func (noopTransportMetrics) ObserveReplyRetryAfter(time.Duration) {}

var (
	_ iris.TransportMetrics = noopTransportMetrics{}
	_ iris.ClientOption     = iris.WithTransportMetrics(noopTransportMetrics{})
)
