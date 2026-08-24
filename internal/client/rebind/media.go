//nolint:wrapcheck // RebindingClient는 활성 APIClient에 위임하는 얇은 shim이라 transport 오류를 그대로 전달한다.
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
