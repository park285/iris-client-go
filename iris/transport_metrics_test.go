package iris_test

import "github.com/park285/iris-client-go/iris"

var _ iris.TransportMetrics = iris.NoopTransportMetrics{}
var _ iris.ClientOption = iris.WithTransportMetrics(iris.NoopTransportMetrics{})
