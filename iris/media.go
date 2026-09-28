package iris

import client "github.com/park285/iris-client-go/v3/internal/client/transport"

type (
	MediaChunkRequest  = client.MediaChunkRequest
	MediaChunkResponse = client.MediaChunkResponse
	MediaClient        = client.MediaClient
)

const PathMediaChunk = client.PathMediaChunk

var (
	_ MediaClient = (*APIClient)(nil)
	_ MediaClient = (*RebindingClient)(nil)
)
