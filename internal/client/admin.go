package client

import (
	"context"

	"github.com/park285/iris-client-go/internal/jsonx"
)

type AdminClient interface {
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
	UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error)
	GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error)
	GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error)
	GetRuntimeDiagnostics(ctx context.Context) (jsonx.RawMessage, error)
	GetChatroomFields(ctx context.Context, chatID int64) (jsonx.RawMessage, error)
	OpenChatroom(ctx context.Context, chatID int64) (jsonx.RawMessage, error)
	GetTextPingDiagnostics(ctx context.Context, chatID int64) (jsonx.RawMessage, error)
	WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error)
}
