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
		h.sdkCtx = ctx
	}
}

type SDKConfig struct {
	Token  string
	Logger *slog.Logger
	Ctx    context.Context
}

func ResolveSDKConfig(opts []HandlerOption) SDKConfig {
	var h Handler
	for _, opt := range opts {
		if opt != nil {
			opt(&h)
		}
	}
	return SDKConfig{Token: h.sdkToken, Logger: h.sdkLogger, Ctx: h.sdkCtx}
}
