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

// WithWebhookToken은 webhook HMAC 비밀키다. 비면 NewDurableHandler가 오류를 반환한다.
func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.token = token
	}
}

// WithWebhookLogger는 handler logger다. Nil이면 slog.Default()를 쓴다.
func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithContext는 handler 수명 context의 값을 제공하는 startup context다. 취소는 물려받지 않으며,
// nil이면 context.Background()를 쓴다.
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

// WithNonceStore는 HMAC replay 방지용 set-once 저장소를 지정합니다. Set-once 마커가 없는 저장소는
// 컴파일 시점에 거절됩니다.
func WithNonceStore(store SetOnceNonceStore) HandlerOption {
	return func(h *Handler) {
		if store != nil {
			h.nonceStore = store
		}
	}
}

// WithAdmitTimeout은 0 이하를 "무제한"이 아니라 기본값 30초로 정규화합니다.
func WithAdmitTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.admitTimeout = d
	}
}

// WithMaxBodyBytes는 요청 본문 상한이다. 0 이하는 기본값 1 MiB로 정규화합니다.
func WithMaxBodyBytes(n int64) HandlerOption {
	return func(h *Handler) {
		h.options.maxBodyBytes = n
	}
}

// WithNonceTimeout은 nonce 저장소 왕복 하나의 상한입니다. 0 이하는 무제한이 아니라 기본값 200ms로 정규화됩니다.
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
