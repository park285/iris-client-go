package webhook

import (
	"context"
	"log/slog"

	base "github.com/park285/iris-client-go/webhook"
)

// Handler는 Iris webhook handler 타입입니다.
type Handler = base.Handler

// HandlerOption은 webhook handler 옵션입니다.
type HandlerOption = base.HandlerOption

// MessageHandler는 webhook 메시지 소비 인터페이스입니다.
type MessageHandler = base.MessageHandler

// Message는 webhook 표준 메시지 타입입니다.
type Message = base.Message

// MessageJSON은 webhook message JSON payload 타입입니다.
type MessageJSON = base.MessageJSON

// WebhookRequest는 webhook 요청 DTO입니다.
type WebhookRequest = base.WebhookRequest

// Metrics는 webhook metrics 인터페이스입니다.
type Metrics = base.Metrics

// Deduplicator는 webhook dedup 인터페이스입니다.
type Deduplicator = base.Deduplicator

// NoopMetrics는 기본 no-op metrics 구현입니다.
type NoopMetrics = base.NoopMetrics

// NoopDeduplicator는 기본 no-op deduplicator 구현입니다.
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

// NewHandler는 Iris webhook handler를 생성합니다.
func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	return base.NewHandler(ctx, token, handler, logger, opts...)
}
