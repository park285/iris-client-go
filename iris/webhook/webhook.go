package webhook

import (
	"context"
	"log/slog"

	base "github.com/park285/iris-client-go/webhook"
)

type Handler = base.Handler
type HandlerOption = base.HandlerOption
type MessageHandler = base.MessageHandler
type Message = base.Message
type MessageJSON = base.MessageJSON
type WebhookRequest = base.WebhookRequest
type Metrics = base.Metrics
type Deduplicator = base.Deduplicator
type NoopMetrics = base.NoopMetrics
type NoopDeduplicator = base.NoopDeduplicator

const (
	PathWebhook         = base.PathWebhook
	HeaderIrisToken     = base.HeaderIrisToken
	HeaderIrisMessageID = base.HeaderIrisMessageID
	DefaultDedupTTL     = base.DefaultDedupTTL
)

var (
	WithMetrics         = base.WithMetrics
	WithDeduplicator    = base.WithDeduplicator
	WithWorkerCount     = base.WithWorkerCount
	WithQueueSize       = base.WithQueueSize
	WithEnqueueTimeout  = base.WithEnqueueTimeout
	WithHandlerTimeout  = base.WithHandlerTimeout
	WithRequireHTTP2    = base.WithRequireHTTP2
	WithDedupTTL        = base.WithDedupTTL
	WithDedupTimeout    = base.WithDedupTimeout
	WithMaxBodyBytes    = base.WithMaxBodyBytes
	WithAutoWorkerCount = base.WithAutoWorkerCount
	ResolveThreadID     = base.ResolveThreadID
	DedupKey            = base.DedupKey
)

func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	return base.NewHandler(ctx, token, handler, logger, opts...)
}
