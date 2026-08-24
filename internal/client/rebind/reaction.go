//nolint:wrapcheck // RebindingClient는 활성 APIClient에 위임하는 얇은 shim이라 transport 오류를 그대로 전달한다.
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
