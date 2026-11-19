package webhook

import (
	"context"
	"log/slog"
	"time"
)

type handlerOptions struct {
	admitTimeout time.Duration
	nonceTimeout time.Duration
	maxBodyBytes int64
}

type HandlerOption func(*Handler)

func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.token = token
	}
}

func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

func WithContext(ctx context.Context) HandlerOption {
	return func(h *Handler) {
		h.baseCtx = ctx //nolint:fatcontext // 옵션으로 받은 context를 보관만 하고 파생하지 않는다.
	}
}

func WithMetrics(m Metrics) HandlerOption {
	return func(h *Handler) {
		if m != nil {
			h.metrics = m
		}
	}
}

func WithNonceStore(store SetOnceNonceStore) HandlerOption {
	return func(h *Handler) {
		if store != nil {
			h.nonceStore = store
		}
	}
}

func WithAdmitTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.admitTimeout = d
	}
}

func WithMaxBodyBytes(n int64) HandlerOption {
	return func(h *Handler) {
		h.options.maxBodyBytes = n
	}
}

func WithNonceTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.nonceTimeout = d
	}
}

func resolveLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.Default()
}

func defaultHandlerOptions() handlerOptions {
	return handlerOptions{
		admitTimeout: defaultAdmitTimeout,
		nonceTimeout: defaultNonceTimeout,
		maxBodyBytes: defaultMaxBodyBytes,
	}
}

func normalizeHandlerOptions(opts handlerOptions) handlerOptions {
	if opts.admitTimeout <= 0 {
		opts.admitTimeout = defaultAdmitTimeout
	}

	if opts.nonceTimeout <= 0 {
		opts.nonceTimeout = defaultNonceTimeout
	}

	if opts.maxBodyBytes <= 0 {
		opts.maxBodyBytes = defaultMaxBodyBytes
	}

	return opts
}
