package rebind

import (
	"context"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
)

var _ transport.ReactionClient = (*RebindingClient)(nil)

func (c *RebindingClient) SendReaction(ctx context.Context, room int64, req transport.ReactionRequest) (*transport.ReactionResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return client.SendReaction(ctx, room, req)
}
