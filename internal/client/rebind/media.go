package rebind

import (
	"context"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
)

var _ transport.MediaClient = (*RebindingClient)(nil)

func (c *RebindingClient) FetchMediaChunk(ctx context.Context, req transport.MediaChunkRequest) (*transport.MediaChunkResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return client.FetchMediaChunk(ctx, req)
}
