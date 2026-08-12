package rebind

import "context"

var _ MediaClient = (*RebindingClient)(nil)

func (c *RebindingClient) FetchMediaChunk(ctx context.Context, req MediaChunkRequest) (*MediaChunkResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return client.FetchMediaChunk(ctx, req)
}
