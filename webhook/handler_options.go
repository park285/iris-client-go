package webhook

import (
	"log/slog"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v2/internal/irishmac"
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

func WithMessageDeduplicator(d MessageDeduplicator) HandlerOption {
	return func(h *Handler) {
		if d != nil {
			h.messageDeduplicator = d
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

// WithAdmitTimeout은 0 이하를 "무제한"이 아니라 기본값 30초로 정규화합니다.
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

// WithDedupPendingTTL은 MessageDeduplicator 예약(pending)의 수명을 지정합니다.
// 확정(commit)된 키는 WithDedupTTL을 따르며, 이 값은 예약 후 확정 전에 프로세스가 죽었을 때
// 키가 pending으로 묶여 있는 최대 시간입니다. 그동안 재전송은 503을 받으므로
// `DedupPendingTTL + 여유 < 발신자에게 남은 재시도 예산`이 성립해야 재전송이 유실되지 않습니다.
func WithDedupPendingTTL(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.dedupPendingTTL = d
	}
}

// WithDedupTimeout은 dedup/nonce 저장소 왕복 하나의 상한입니다. 0 이하는 무제한이 아니라 기본값으로 정규화됩니다.
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

// WithNonceStore는 HMAC replay 방지용 set-once 저장소를 지정합니다.
func WithNonceStore(store NonceStore) HandlerOption {
	return func(h *Handler) {
		if store != nil {
			h.nonceStore = store
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

func (h *Handler) resolveDedupPendingObserver() {
	if observer, ok := h.metrics.(DedupPendingObserver); ok {
		h.dedupPendingObserver = observer
	}
}

// durable admitter는 handleDedupKey를 거치지 않아 예약 자체가 없으므로, 예약 수명에 관한
// 경고는 모두 이 경로에만 해당한다.
func (h *Handler) usesDedupReservations() bool {
	return h.admitter == nil && h.messageDeduplicator != nil
}

func (h *Handler) usesMessageDedup() bool {
	return h.admitter == nil && h.messageDeduplicator != nil
}

func (h *Handler) warnDedupConfiguration(requestedPendingTTL time.Duration) {
	if requestedPendingTTL > 0 && requestedPendingTTL > h.options.DedupTTL {
		h.logger.Warn(
			"webhook dedup pending TTL exceeds the committed TTL and was clamped; a pending reservation must expire well before the sender stops retrying",
			slog.Duration("requestedPendingTTL", requestedPendingTTL),
			slog.Duration("effectivePendingTTL", h.dedupPendingTTL),
			slog.Duration("dedupTTL", h.options.DedupTTL),
		)
	}

	if h.usesMessageDedup() && h.options.DedupTTL <= senderMaxDeliveryHorizon {
		h.logger.Warn(
			"webhook dedup TTL does not outlive the sender's maximum delivery horizon; a committed dedup key can expire before the final attempt completes, so the same message can be processed again",
			slog.Duration("dedupTTL", h.options.DedupTTL),
			slog.Duration("senderMaxDeliveryHorizon", senderMaxDeliveryHorizon),
		)
	}

	if h.usesDedupReservations() && h.dedupPendingTTL >= senderFinalRetryWaitFloor {
		h.logger.Warn(
			"webhook dedup pending TTL is not shorter than the shortest wait before the sender's last retry; a reservation left behind by a crash during the final retryable attempt outlives that retry, so that message is lost instead of retransmitted",
			slog.Duration("pendingTTL", h.dedupPendingTTL),
			slog.Duration("senderFinalRetryWaitFloor", senderFinalRetryWaitFloor),
		)
	}

	if h.usesDedupReservations() && h.options.EnqueueTimeout+2*h.options.DedupTimeout >= h.dedupPendingTTL {
		h.logger.Warn(
			"webhook enqueue timeout plus the reserve and commit dedup round trips is not shorter than the dedup pending TTL; a reservation can expire while the request is still enqueuing or committing, making every Commit fail as a lost reservation",
			slog.Duration("enqueueTimeout", h.options.EnqueueTimeout),
			slog.Duration("dedupTimeout", h.options.DedupTimeout),
			slog.Duration("pendingTTL", h.dedupPendingTTL),
		)
	}
}

func isSetOnceNonceStore(d NonceStore) bool {
	_, ok := d.(SetOnceNonceStore)

	return ok
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
		AdmitTimeout:   defaultAdmitTimeout,
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

	if opts.AdmitTimeout <= 0 {
		opts.AdmitTimeout = defaultAdmitTimeout
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

func normalizeDedupPendingTTL(pendingTTL, dedupTTL time.Duration) time.Duration {
	if pendingTTL <= 0 {
		pendingTTL = defaultDedupPendingTTL
	}

	return min(pendingTTL, dedupTTL)
}
