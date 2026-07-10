package iris_test

import (
	"time"

	"github.com/park285/iris-client-go/iris"
)

var (
	_ iris.BotClient = (*iris.H2CClient)(nil)
	_ iris.Client    = (*iris.H2CClient)(nil)

	_ iris.BotClient    = (*iris.RebindingClient)(nil)
	_ iris.Client       = (*iris.RebindingClient)(nil)
	_ iris.KaringClient = (*iris.RebindingClient)(nil)

	_ = iris.RebindingClientConfig{ResolveInterval: time.Second}
)
