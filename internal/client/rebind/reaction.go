package rebind

import "context"

var _ ReactionClient = (*RebindingClient)(nil)

func (c *RebindingClient) SendReaction(ctx context.Context, room int64, req ReactionRequest) (*ReactionResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return client.SendReaction(ctx, room, req)
}
