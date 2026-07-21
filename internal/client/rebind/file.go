package rebind

import (
	"context"

	"github.com/park285/iris-client-go/internal/client/transport"
)

type ReplyFile = transport.ReplyFile

var _ transport.FileSender = (*RebindingClient)(nil)

func (c *RebindingClient) SendFile(
	ctx context.Context,
	room string,
	file ReplyFile,
	opts ...SendOption,
) (*ReplyAcceptedResponse, error) {
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
	opts ...SendOption,
) (*ReplyAcceptedResponse, error) {
	client, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return client.SendFilePath(ctx, room, path, contentType, opts...)
}
