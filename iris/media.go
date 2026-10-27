package iris

import client "github.com/park285/iris-client-go/v3/internal/client/transport"

type (
	MediaChunkRequest  = client.MediaChunkRequest
	MediaChunkResponse = client.MediaChunkResponse
	MediaClient        = client.MediaClient
)

var _ MediaClient = (*APIClient)(nil)
