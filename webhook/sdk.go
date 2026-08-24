package webhook

import (
	"context"
	"log/slog"
)

func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.sdkToken = token
	}
}

func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.sdkLogger = logger
	}
}

func WithContext(ctx context.Context) HandlerOption {
	return func(h *Handler) {
		h.sdkCtx = ctx //nolint:fatcontext // 옵션으로 받은 context를 보관만 하고 파생하지 않는다.
	}
}

type SDKConfig struct {
	Token  string
	Secret string
	Logger *slog.Logger
	Ctx    context.Context //nolint:containedctx // WithContext 옵션 값을 그대로 노출하는 SDK 설정 스냅샷이다.
}

func ResolveSDKConfig(opts []HandlerOption) SDKConfig {
	var h Handler

	for _, opt := range opts {
		if opt != nil {
			opt(&h)
		}
	}

	return SDKConfig{Token: h.sdkToken, Secret: h.webhookSecret, Logger: h.sdkLogger, Ctx: h.sdkCtx}
}
