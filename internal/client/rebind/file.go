//nolint:wrapcheck // RebindingClient는 활성 APIClient에 위임하는 얇은 shim이라 transport 오류를 그대로 전달한다.
package rebind

import (
	"context"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
)

var _ transport.FileSender = (*RebindingClient)(nil)

func (c *RebindingClient) SendFile(
	ctx context.Context,
	room string,
	file transport.ReplyFile,
	opts ...transport.SendOption,
) (*transport.ReplyAcceptedResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}

	return client.SendFile(ctx, room, file, opts...)
}

func (c *RebindingClient) SendFilePath(
	ctx context.Context,
	room string,
	path string,
	contentType string,
	opts ...transport.SendOption,
) (*transport.ReplyAcceptedResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}

	return client.SendFilePath(ctx, room, path, contentType, opts...)
}
