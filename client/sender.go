package client

import "context"

// Sender is the message-sending interface for Iris.
type Sender interface {
	SendMessage(ctx context.Context, room, message string, opts ...SendOption) error
	SendImage(ctx context.Context, room, imageBase64 string, opts ...SendOption) error
	SendMultipleImages(ctx context.Context, room string, imageBase64s []string, opts ...SendOption) error
}
