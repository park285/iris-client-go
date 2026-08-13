package webhook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const (
	defaultWorkerCount    = 16
	defaultQueueSize      = 1000
	defaultEnqueueTimeout = 50 * time.Millisecond
	defaultHandlerTimeout = 30 * time.Second
	defaultDedupTimeout   = 200 * time.Millisecond
	defaultAdmitTimeout   = 30 * time.Second
	// 아래 값은 Iris webhook worker의 상수를 인코딩한 것이라 이 저장소 코드만으로는 유도할
	// 수 없다. delivery.max_attempts와 request_timeout_ms는 iris-runtime의
	// runtime/env/worker_profile/defaults.rs, max wait와 breaker cooldown은 webhook/retry.rs와
	// circuit_breaker.rs다.
	senderMaxAttempts             = 6
	senderAttemptTimeout          = 125 * time.Second
	senderMaxWait                 = 30 * time.Second
	senderBreakerFailureThreshold = 5
	senderBreakerCooldown         = 30 * time.Second
	// 예약이 남는 시점은 프로세스가 죽은 그 attempt이므로, 비교 대상은 첫 시도부터의 전체
	// 지평(하한 24.8s)이 아니라 그 시점에 남은 재시도 예산이다. 최악은 마지막 재시도 가능
	// attempt(4)에서 죽는 경우이고, 남은 것은 base 16s에 -20% jitter가 걸린 대기 한 번뿐이다.
	senderFinalRetryWaitFloor = 12800 * time.Millisecond
	// breaker Deferred도 attempt budget을 소비한다. Iris의 절대 전송 상한은 모든 attempt의
	// request timeout과 attempt 사이의 더 큰 wait(retry cap 또는 breaker cooldown)를 합친 값이다.
	senderMaxDeliveryHorizon = senderMaxAttempts*senderAttemptTimeout +
		(senderMaxAttempts-1)*max(senderMaxWait, senderBreakerCooldown)
	defaultDedupPendingTTL = 5 * time.Second
	defaultMaxBodyBytes    = 1 << 20
	// replayWindow(5m) < senderMaxDeliveryHorizon(15m)이고 인증 성공 요청은 응답이 400/503이어도
	// nonce가 set-once로 소모되지만, Iris sender는 attempt마다 요청을 새로 서명한다
	// (iris-runtime/src/webhook: send_permitted_delivery → signing_for_delivery가 매 attempt
	// new_webhook_nonce()·fresh timestamp 생성). 이 전제가 깨지면 503 후 재전송이 duplicate
	// nonce 401로 죽어 ec35264의 유실 차단이 무효화되므로, Iris 서명 경로를 바꿀 때는 이 계약을 함께 봐야 한다.
	defaultReplayWindow  = 5 * time.Minute
	maxEventPayloadBytes = 256 << 10
)

var (
	errQueueFull = errors.New("webhook queue full")
	errClosed    = errors.New("webhook handler closed")
)

var ErrMessageAdmitterRequired = errors.New("webhook: message admitter is required")

// MessageHandler는 수신된 webhook 메시지를 처리하는 인터페이스입니다.
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message)
}

// MessageAdmitter는 HTTP 200 전에 메시지를 durable store에 commit하는 계약이다.
type MessageAdmitter interface {
	AdmitMessage(ctx context.Context, msg *Message) error
}

type TaskPool interface {
	SubmitWait(task func()) bool
}

type ReceiveDiagnostics struct {
	WorkersConfigured int    `json:"workersConfigured"`
	QueueSize         int    `json:"queueSize"`
	Pending           int    `json:"pending"`
	InFlight          int    `json:"inFlight"`
	EnqueueRejected   uint64 `json:"enqueueRejected"`
	QueueFullCount    uint64 `json:"queueFullCount"`
	HandlerTimeouts   uint64 `json:"handlerTimeoutCount"`
}

// Handler는 stripe 워커 풀을 갖춘 webhook HTTP 핸들러입니다.
type Handler struct {
	token                string
	webhookSecret        string
	replayWindow         time.Duration
	nonceCache           NonceStore
	nonceCacheExplicit   bool
	nonceCacheFellBack   bool
	webhookSigner        *irishmac.Signer
	handler              MessageHandler
	admitter             MessageAdmitter
	dedup                Deduplicator
	statefulDedup        StatefulDeduplicator
	logger               *slog.Logger
	metrics              Metrics
	dedupPendingObserver DedupPendingObserver
	options              HandlerOptions
	dedupPendingTTL      time.Duration
	baseCtxFn            func() context.Context

	// SDK 수준 필드: iris.NewWebhookHandler에서만 사용되며 NewHandler에서는 무시됩니다.
	sdkToken  string
	sdkLogger *slog.Logger
	sdkCtx    context.Context

	queueLock sync.RWMutex
	closed    bool
	closedCh  chan struct{}
	enqueueWG sync.WaitGroup
	sched     *scheduler
	taskPool  TaskPool
	ownsPool  bool
	runCtx    context.Context
	runCancel context.CancelFunc
	closeOnce sync.Once
	closeDone chan struct{}

	activeTasks           atomic.Int32
	enqueueRejected       atomic.Uint64
	queueFull             atomic.Uint64
	handlerTimeouts       atomic.Uint64
	dedupPendingRejected  atomic.Uint64
	nonceStoreUnavailable atomic.Uint64
}

type webhookTask struct {
	msg *Message
}

func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	result := &Handler{
		token:      strings.TrimSpace(token),
		handler:    handler,
		dedup:      NoopDeduplicator{},
		nonceCache: newMemoryNonceCache(),
		logger:     resolveLogger(logger),
		metrics:    NoopMetrics{},
		options:    defaultHandlerOptions(),
		baseCtxFn:  contextSource(ctx),
		closedCh:   make(chan struct{}),
		closeDone:  make(chan struct{}),
	}
	result.webhookSecret = result.token

	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}
	if result.handler != nil && result.admitter != nil {
		result.logger.Warn("webhook message handler is not invoked in durable admission mode; dispatch admitted messages from the consumer inbox loop or use NewDurableHandler")
	}

	requestedPendingTTL := result.dedupPendingTTL
	result.options = normalizeHandlerOptions(result.options)
	result.dedupPendingTTL = normalizeDedupPendingTTL(result.dedupPendingTTL, result.options.DedupTTL)
	result.normalizeHMACOptions()
	result.resolveNonceCacheBackend()
	result.resolveStatefulDedup()
	result.resolveDedupPendingObserver()
	result.warnDedupConfiguration(requestedPendingTTL)
	// HTTP receive context는 decode/admission까지만 소유한다. 실행 context는 startup
	// snapshot의 값을 보존하되 shutdown이 시작될 때만 취소한다.
	result.runCtx, result.runCancel = context.WithCancel(context.WithoutCancel(result.baseContext()))
	if result.admitter != nil {
		return result
	}
	if result.taskPool == nil {
		result.taskPool = newInternalPool(result.options.WorkerCount, 0)
		result.ownsPool = true
	}
	result.sched = newScheduler(result.options.QueueSize, result.taskPool, result.options.OrderingMode, result.logger)
	result.sched.start(result.options.WorkerCount, result.makeTaskRunner(result.runCtx))

	return result
}

// NewDurableHandler는 MessageHandler 없이 durable admission 전용 Handler를 구성한다.
// 처리(dispatch)는 소비자의 inbox 루프가 소유한다.
func NewDurableHandler(
	ctx context.Context,
	token string,
	admitter MessageAdmitter,
	logger *slog.Logger,
	opts ...HandlerOption,
) (*Handler, error) {
	if admitter == nil {
		return nil, ErrMessageAdmitterRequired
	}

	merged := make([]HandlerOption, 0, len(opts)+1)
	merged = append(merged, opts...)
	merged = append(merged, WithDurableAdmission(admitter))

	return NewHandler(ctx, token, nil, logger, merged...), nil
}

// Close는 admission을 닫고 모든 작업이 끝날 때까지 기다리는 호환 wrapper입니다.
func (h *Handler) Close() error {
	return h.CloseContext(context.Background())
}

// CloseContext는 grace context가 끝나면 queued callback을 건너뛰고 in-flight context를 취소한다.
func (h *Handler) CloseContext(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.beginClose()
	select {
	case <-h.closeDone:
		return nil
	default:
	}
	select {
	case <-h.closeDone:
		return nil
	case <-ctx.Done():
		if h.runCancel != nil {
			h.runCancel()
		}
		select {
		case <-h.closeDone:
			return nil
		default:
		}
		return ctx.Err()
	}
}

func (h *Handler) beginClose() {
	h.closeOnce.Do(func() {
		h.queueLock.Lock()
		h.closed = true
		close(h.closedCh)
		h.queueLock.Unlock()

		go func() {
			h.enqueueWG.Wait()
			if h.sched != nil {
				h.sched.close()
			}
			if h.ownsPool {
				if stopper, ok := h.taskPool.(interface{ StopAndWait() }); ok {
					stopper.StopAndWait()
				}
			}
			if h.runCancel != nil {
				h.runCancel()
			}
			close(h.closeDone)
		}()
	})
}

// ServeHTTP는 Iris webhook 요청을 처리합니다.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.metrics.ObserveRequest()
	if !h.acceptTransport(w, r) {
		return
	}

	req, ok := h.decodeAndValidate(w, r)
	if !ok {
		return
	}
	if !h.reconcileMessageID(w, r, req) {
		return
	}

	var reservation dedupReservation
	if h.admitter == nil {
		handled, reserved := h.handleDedupKey(w, r, canonicalDedupID(req))
		if handled {
			return
		}
		reservation = reserved
	}

	msg := buildMessage(req)
	if h.admitter != nil {
		// durable 경로는 큐 dispatch(runTask)를 거치지 않으므로 handler duration을 여기서 직접 관측한다.
		admitStart := time.Now()
		err := h.admitMessage(r.Context(), msg)
		h.metrics.ObserveHandlerDuration(time.Since(admitStart))
		if err != nil {
			h.enqueueRejected.Add(1)
			h.metrics.ObserveEnqueueFailure()
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		h.metrics.ObserveAccepted()
		w.WriteHeader(http.StatusOK)

		return
	}
	if err := h.enqueueTask(r.Context(), webhookTask{msg: msg}); err != nil {
		h.enqueueRejected.Add(1)
		if errors.Is(err, errQueueFull) {
			h.queueFull.Add(1)
		}
		h.releaseDedupKey(r.Context(), reservation)
		h.metrics.ObserveEnqueueFailure()
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	h.commitDedupReservation(r.Context(), reservation)
	h.metrics.ObserveAccepted()
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Diagnostics() ReceiveDiagnostics {
	if h == nil {
		return ReceiveDiagnostics{}
	}
	pending := 0
	if h.sched != nil {
		pending = int(h.sched.depth.Load())
	}
	return ReceiveDiagnostics{
		WorkersConfigured: h.options.WorkerCount,
		QueueSize:         h.options.QueueSize,
		Pending:           pending,
		InFlight:          int(h.activeTasks.Load()),
		EnqueueRejected:   h.enqueueRejected.Load(),
		QueueFullCount:    h.queueFull.Load(),
		HandlerTimeouts:   h.handlerTimeouts.Load(),
	}
}

// DedupPendingRejectedCount는 확정 전 예약 때문에 503으로 되돌린 요청 수를 반환합니다.
func (h *Handler) DedupPendingRejectedCount() uint64 {
	if h == nil {
		return 0
	}

	return h.dedupPendingRejected.Load()
}

// NonceStoreUnavailableCount는 HMAC nonce 저장소 조회 실패(오류·타임아웃) 때문에 503으로
// 되돌린 요청 수를 반환합니다.
func (h *Handler) NonceStoreUnavailableCount() uint64 {
	if h == nil {
		return 0
	}

	return h.nonceStoreUnavailable.Load()
}

func contextSource(ctx context.Context) func() context.Context {
	if ctx == nil {
		return context.Background
	}

	return func() context.Context {
		return ctx
	}
}

func (h *Handler) baseContext() context.Context {
	if h.baseCtxFn == nil {
		return context.Background()
	}

	return h.baseCtxFn()
}
