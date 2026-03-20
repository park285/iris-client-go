package client

import (
	"context"

	iris "park285/iris-client-go"
)

// Sender is the message-sending interface for Iris.
type Sender interface {
	SendMessage(ctx context.Context, room, message string, opts ...iris.SendOption) error
	SendImage(ctx context.Context, room, imageBase64 string) error
}
