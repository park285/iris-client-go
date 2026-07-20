package webhook

import (
	"log/slog"
	"strings"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

type HandlerOptions struct {
	WorkerCount    int
	QueueSize      int
	EnqueueTimeout time.Duration
	AdmitTimeout   time.Duration
	HandlerTimeout time.Duration
	OrderingMode   OrderingMode
	DedupTTL       time.Duration
	DedupTimeout   time.Duration
	MaxBodyBytes   int64
}

type OrderingMode int

const (
	OrderingModeKey OrderingMode = iota
	OrderingModeNone
)

type HandlerOption func(*Handler)

func WithMetrics(m Metrics) HandlerOption {
	return func(h *Handler) {
		if m != nil {
			h.metrics = m
		}
	}
}

func WithDeduplicator(d Deduplicator) HandlerOption {
	return func(h *Handler) {
		if d != nil {
			h.dedup = d
		}
	}
}

func WithTaskPool(pool TaskPool) HandlerOption {
	return func(h *Handler) {
		h.taskPool = pool
	}
}

// WithDurableAdmission은 in-memory scheduler 대신 동기 durable admission을 사용한다.
func WithDurableAdmission(admitter MessageAdmitter) HandlerOption {
	return func(h *Handler) {
		h.admitter = admitter
	}
}

func WithWorkerCount(n int) HandlerOption {
	return func(h *Handler) {
		h.options.WorkerCount = n
	}
}

func WithQueueSize(n int) HandlerOption {
	return func(h *Handler) {
		h.options.QueueSize = n
	}
}

func WithEnqueueTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.EnqueueTimeout = d
	}
}

func WithAdmitTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.AdmitTimeout = d
	}
}

func WithHandlerTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.HandlerTimeout = d
	}
}

func WithOrderingMode(mode OrderingMode) HandlerOption {
	return func(h *Handler) {
		h.options.OrderingMode = mode
	}
}

func WithDedupTTL(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.DedupTTL = d
	}
}

func WithDedupTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.DedupTimeout = d
	}
}

func WithMaxBodyBytes(n int64) HandlerOption {
	return func(h *Handler) {
		h.options.MaxBodyBytes = n
	}
}

func WithWebhookSecret(secret string) HandlerOption {
	return func(h *Handler) {
		h.webhookSecret = strings.TrimSpace(secret)
	}
}

func WithReplayWindow(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.replayWindow = d
	}
}

func WithNonceCache(store Deduplicator) HandlerOption {
	return func(h *Handler) {
		if store != nil {
			h.nonceCache = store
			h.nonceCacheExplicit = true
		}
	}
}

func (h *Handler) normalizeHMACOptions() {
	h.webhookSecret = strings.TrimSpace(h.webhookSecret)
	if h.webhookSecret == "" {
		h.webhookSecret = h.token
	}
	if h.replayWindow <= 0 {
		h.replayWindow = defaultReplayWindow
	}
	h.webhookSigner = irishmac.NewSigner(h.webhookSecret)
}

// dedup 키(iris:msg:{id})와 nonce 키(METHOD\n...)는 disjoint하고 백엔드는 호출별 TTL을
// 적용하므로 공유가 안전하다. Noop은 공유하면 replay 보호가 무력화되므로 제외한다.
func (h *Handler) resolveNonceCacheBackend() {
	if h.nonceCacheExplicit {
		return
	}
	if h.dedup != nil && !isNoopDeduplicator(h.dedup) {
		h.nonceCache = h.dedup
	}
}

func isNoopDeduplicator(d Deduplicator) bool {
	switch d.(type) {
	case NoopDeduplicator, *NoopDeduplicator:
		return true
	default:
		return false
	}
}

func resolveLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.Default()
}

func defaultHandlerOptions() HandlerOptions {
	return HandlerOptions{
		WorkerCount:    defaultWorkerCount,
		QueueSize:      defaultQueueSize,
		EnqueueTimeout: defaultEnqueueTimeout,
		HandlerTimeout: defaultHandlerTimeout,
		DedupTTL:       DefaultDedupTTL,
		DedupTimeout:   defaultDedupTimeout,
		MaxBodyBytes:   defaultMaxBodyBytes,
	}
}

func normalizeHandlerOptions(opts HandlerOptions) HandlerOptions {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = defaultWorkerCount
	}

	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultQueueSize
	}

	if opts.EnqueueTimeout <= 0 {
		opts.EnqueueTimeout = defaultEnqueueTimeout
	}

	if opts.HandlerTimeout <= 0 {
		opts.HandlerTimeout = defaultHandlerTimeout
	}

	if opts.OrderingMode != OrderingModeKey && opts.OrderingMode != OrderingModeNone {
		opts.OrderingMode = OrderingModeKey
	}

	if opts.DedupTTL <= 0 {
		opts.DedupTTL = DefaultDedupTTL
	}

	if opts.DedupTimeout <= 0 {
		opts.DedupTimeout = defaultDedupTimeout
	}

	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = defaultMaxBodyBytes
	}

	return opts
}
