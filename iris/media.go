package iris

import (
	"github.com/park285/iris-client-go/v6/internal/client/common"
	client "github.com/park285/iris-client-go/v6/internal/client/transport"
)

type (
	MediaChunkRequest  = common.MediaChunkRequest
	MediaChunkResponse = common.MediaChunkResponse
	MediaClient        = client.MediaClient
)

var _ MediaClient = (*APIClient)(nil)
