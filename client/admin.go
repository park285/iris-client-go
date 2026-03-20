package client

import "context"

// AdminClient is the admin/utility API interface for Iris.
type AdminClient interface {
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*Config, error)
	Decrypt(ctx context.Context, data string) (string, error)
}
