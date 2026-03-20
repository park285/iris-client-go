package client

import (
	"context"

	iris "park285/iris-client-go"
)

// AdminClient is the admin/utility API interface for Iris.
type AdminClient interface {
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*iris.Config, error)
	Decrypt(ctx context.Context, data string) (string, error)
}
