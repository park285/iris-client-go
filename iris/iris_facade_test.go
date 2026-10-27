package iris_test

import (
	"time"

	"github.com/park285/iris-client-go/v3/iris"
)

var (
	_ iris.BotClient = (*iris.APIClient)(nil)
	_ iris.BotClient = (*iris.RebindingClient)(nil)

	_ = iris.RebindingClientConfig{ResolveInterval: time.Second}
)
