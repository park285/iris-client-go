package iris

import (
	"context"
	"log/slog"

	basewebhook "github.com/park285/iris-client-go/webhook"
)

type WebhookHandler = basewebhook.Handler
type HandlerOption = basewebhook.HandlerOption
type MessageHandler = basewebhook.MessageHandler
type Message = basewebhook.Message
type MessageJSON = basewebhook.MessageJSON
type WebhookRequest = basewebhook.WebhookRequest
type WebhookMention = basewebhook.WebhookMention
type Metrics = basewebhook.Metrics
type Deduplicator = basewebhook.Deduplicator
type TaskPool = basewebhook.TaskPool
type NoopMetrics = basewebhook.NoopMetrics
type NoopDeduplicator = basewebhook.NoopDeduplicator
type HandlerOptions = basewebhook.HandlerOptions
type WebhookOrderingMode = basewebhook.OrderingMode
type WebhookReceiveDiagnostics = basewebhook.ReceiveDiagnostics
type WebhookSDKConfig = basewebhook.SDKConfig

const (
	PathWebhook = basewebhook.PathWebhook

	HeaderIrisToken     = basewebhook.HeaderIrisToken
	HeaderIrisMessageID = basewebhook.HeaderIrisMessageID
	HeaderIrisRoute     = basewebhook.HeaderIrisRoute

	DefaultDedupTTL = basewebhook.DefaultDedupTTL
)

const (
	WebhookOrderingModeKey  WebhookOrderingMode = basewebhook.OrderingModeKey
	WebhookOrderingModeNone WebhookOrderingMode = basewebhook.OrderingModeNone
)

var (
	ResolveWebhookSDKConfig = basewebhook.ResolveSDKConfig

	WithWebhookToken    = basewebhook.WithWebhookToken
	WithWebhookLogger   = basewebhook.WithWebhookLogger
	WithContext         = basewebhook.WithContext
	WithMetrics         = basewebhook.WithMetrics
	WithDeduplicator    = basewebhook.WithDeduplicator
	WithTaskPool        = basewebhook.WithTaskPool
	WithWorkerCount     = basewebhook.WithWorkerCount
	WithQueueSize       = basewebhook.WithQueueSize
	WithEnqueueTimeout  = basewebhook.WithEnqueueTimeout
	WithHandlerTimeout  = basewebhook.WithHandlerTimeout
	WithRequireHTTP2    = basewebhook.WithRequireHTTP2
	WithDedupTTL        = basewebhook.WithDedupTTL
	WithDedupTimeout    = basewebhook.WithDedupTimeout
	WithMaxBodyBytes    = basewebhook.WithMaxBodyBytes
	WithAutoWorkerCount = basewebhook.WithAutoWorkerCount
	ResolveThreadID     = basewebhook.ResolveThreadID
	DedupKey            = basewebhook.DedupKey
)

func WithWebhookOrderingMode(mode WebhookOrderingMode) HandlerOption {
	return basewebhook.WithOrderingMode(mode)
}

func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *WebhookHandler {
	return basewebhook.NewHandler(ctx, token, handler, logger, opts...)
}
