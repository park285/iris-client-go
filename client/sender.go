package client

import "context"

type Sender interface {
	SendMessage(ctx context.Context, room, message string, opts ...SendOption) error
	SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) error
	SendMultipleImages(ctx context.Context, room string, images [][]byte, opts ...SendOption) error
	SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error)
	GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error)
}
