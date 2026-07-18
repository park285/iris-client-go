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
	defaultMaxBodyBytes   = 1 << 20
	defaultReplayWindow   = 5 * time.Minute
	maxEventPayloadBytes  = 256 << 10
	maxMessageIDBytes     = 256
)

var (
	errQueueFull = errors.New("webhook queue full")
	errClosed    = errors.New("webhook handler closed")
)

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
	token              string
	tokenBytes         []byte
	webhookSecret      string
	replayWindow       time.Duration
	nonceCache         Deduplicator
	nonceCacheExplicit bool
	webhookSigner      *irishmac.Signer
	handler            MessageHandler
	admitter           MessageAdmitter
	dedup              Deduplicator
	logger             *slog.Logger
	metrics            Metrics
	options            HandlerOptions
	baseCtxFn          func() context.Context

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

	activeTasks     atomic.Int32
	enqueueRejected atomic.Uint64
	queueFull       atomic.Uint64
	handlerTimeouts atomic.Uint64
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
	result.tokenBytes = []byte(result.token)
	result.webhookSecret = result.token

	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}

	result.options = normalizeHandlerOptions(result.options)
	result.normalizeHMACOptions()
	result.resolveNonceCacheBackend()
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

	var reservedDedupKey string
	if h.admitter == nil {
		duplicate, handled, reserved := h.handleDedupKey(w, r, canonicalDedupID(req))
		if handled {
			if duplicate {
				h.metrics.ObserveDuplicate()
			}

			return
		}
		reservedDedupKey = reserved
	}

	msg := buildMessage(req)
	if h.admitter != nil {
		if err := h.admitMessage(r.Context(), msg); err != nil {
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
		h.releaseDedupKey(r.Context(), reservedDedupKey)
		h.metrics.ObserveEnqueueFailure()
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

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
