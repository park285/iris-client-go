package iris_test

import "github.com/park285/iris-client-go/v2/iris"

var (
	_ iris.TransportMetrics = iris.NoopTransportMetrics{}
	_ iris.ClientOption     = iris.WithTransportMetrics(iris.NoopTransportMetrics{})
)
