package webhook

import (
	"context"
	"log/slog"
)

// WithWebhookToken sets the webhook authentication token. Used by iris.NewWebhookHandler.
func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.sdkToken = token
	}
}

// WithWebhookLogger sets the logger. Used by iris.NewWebhookHandler.
func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.sdkLogger = logger
	}
}

// WithContext sets the base context. Used by iris.NewWebhookHandler.
func WithContext(ctx context.Context) HandlerOption {
	return func(h *Handler) {
		h.sdkCtx = ctx
	}
}

// SDKConfig holds SDK-level settings extracted from HandlerOption.
type SDKConfig struct {
	Token  string
	Logger *slog.Logger
	Ctx    context.Context
}

// ResolveSDKConfig applies options to a zero Handler and extracts SDK fields.
func ResolveSDKConfig(opts []HandlerOption) SDKConfig {
	var h Handler
	for _, opt := range opts {
		if opt != nil {
			opt(&h)
		}
	}
	return SDKConfig{Token: h.sdkToken, Logger: h.sdkLogger, Ctx: h.sdkCtx}
}
