package client

import "context"

// AdminClient is the admin/utility API interface for Iris.
type AdminClient interface {
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
	UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error)
	Decrypt(ctx context.Context, data string) (string, error)
	GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error)
	Query(ctx context.Context, req QueryRequest) (*QueryResponse, error)
}
