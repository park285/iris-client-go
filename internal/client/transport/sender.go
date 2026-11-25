package transport

import (
	"context"

	"github.com/park285/iris-client-go/v6/internal/client/common"
)

type Sender interface {
	SendMessageAccepted(ctx context.Context, room, message string, opts ...SendOption) (*common.ReplyAcceptedResponse, error)
	SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) (*common.ReplyAcceptedResponse, error)
	SendMultipleImages(ctx context.Context, room string, images [][]byte, opts ...SendOption) (*common.ReplyAcceptedResponse, error)
	SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*common.ReplyAcceptedResponse, error)
	GetReplyStatus(ctx context.Context, requestID string) (*common.ReplyStatusSnapshot, error)
}
