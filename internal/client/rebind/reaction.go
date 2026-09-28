package rebind

import (
	"context"

	"github.com/park285/iris-client-go/v3/internal/client/transport"
)

var _ transport.ReactionClient = (*RebindingClient)(nil)

func (c *RebindingClient) SendReaction(ctx context.Context, room int64, req transport.ReactionRequest) (*transport.ReactionResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}

	var reactionClient transport.ReactionClient = client

	return reactionClient.SendReaction(ctx, room, req)
}
