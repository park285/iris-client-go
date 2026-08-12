package iris

import client "github.com/park285/iris-client-go/internal/client/transport"

type MediaChunkRequest = client.MediaChunkRequest
type MediaChunkResponse = client.MediaChunkResponse
type MediaClient = client.MediaClient

const PathMediaChunk = client.PathMediaChunk

var _ MediaClient = (*H2CClient)(nil)
var _ MediaClient = (*RebindingClient)(nil)
