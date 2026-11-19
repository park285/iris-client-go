package webhook

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

const (
	defaultAdmitTimeout  = 30 * time.Second
	defaultNonceTimeout  = 200 * time.Millisecond
	defaultMaxBodyBytes  = 1 << 20
	defaultReplayWindow  = 5 * time.Minute
	maxEventPayloadBytes = 256 << 10
)

var errClosed = errors.New("webhook handler closed")

var (
	ErrMessageAdmitterRequired = errors.New("webhook: message admitter is required")
	ErrNonceStoreRequired      = errors.New("webhook: explicit set-once nonce store is required")
)

var ErrAlreadyAdmitted = errors.New("webhook: message already admitted")

type MessageAdmitter interface {
	AdmitMessage(ctx context.Context, msg *Message, body []byte) error
}

type SignatureVersionDiagnostics struct {
	V3Validated       uint64 `json:"v3Validated"`
	UnknownRejected   uint64 `json:"unknownRejected"`
	MalformedRejected uint64 `json:"malformedRejected"`
}

type Handler struct {
	token         string
	replayWindow  time.Duration
	nonceStore    SetOnceNonceStore
	webhookSigner *irishmac.Signer
	admitter      MessageAdmitter
	logger        *slog.Logger
	metrics       Metrics
	options       handlerOptions
	baseCtx       context.Context //nolint:containedctx // WithContext 옵션 값을 생성 시점의 run context 파생까지만 보관한다.

	admissionLock sync.RWMutex
	closed        bool
	closedCh      chan struct{}
	admissions    sync.WaitGroup
	runCtx        context.Context //nolint:containedctx // 핸들러 수명 동안 admission 취소를 묶는 run context다.
	runCancel     context.CancelFunc
	closeOnce     sync.Once
	closeDone     chan struct{}

	signatureV3Validated atomic.Uint64
	signatureUnknown     atomic.Uint64
	signatureMalformed   atomic.Uint64
}

func NewDurableHandler(admitter MessageAdmitter, opts ...HandlerOption) (*Handler, error) {
	if admitter == nil {
		return nil, ErrMessageAdmitterRequired
	}

	h := &Handler{
		replayWindow: defaultReplayWindow,
		admitter:     admitter,
		metrics:      NoopMetrics{},
		options:      defaultHandlerOptions(),
		closedCh:     make(chan struct{}),
		closeDone:    make(chan struct{}),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}

	h.token = strings.TrimSpace(h.token)
	if h.token == "" {
		return nil, errors.New("iris: webhook token is required (use webhook.WithWebhookToken)")
	}

	if h.nonceStore == nil {
		return nil, fmt.Errorf("iris: new durable webhook handler: %w", ErrNonceStoreRequired)
	}

	h.logger = resolveLogger(h.logger)
	h.options = normalizeHandlerOptions(h.options)
	h.webhookSigner = irishmac.NewSigner(h.token)

	runCtx, runCancel := context.WithCancel(context.WithoutCancel(cmp.Or(h.baseCtx, context.Background())))

	h.runCtx, h.runCancel = runCtx, runCancel //nolint:fatcontext // 생성 시 한 번만 파생하는 핸들러 수명 context다.

	return h, nil
}

func (h *Handler) Close() error {
	return h.CloseContext(context.Background())
}

func (h *Handler) CloseContext(ctx context.Context) error {
	if h == nil {
		return nil
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

		return fmt.Errorf("close webhook handler: %w", ctx.Err())
	}
}

func (h *Handler) beginClose() {
	h.closeOnce.Do(func() {
		h.admissionLock.Lock()

		h.closed = true
		close(h.closedCh)
		h.admissionLock.Unlock()

		go func() {
			h.admissions.Wait()

			if h.runCancel != nil {
				h.runCancel()
			}

			close(h.closeDone)
		}()
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.metrics.ObserveRequest()

	body, ok := h.acceptTransport(w, r)
	if !ok {
		return
	}

	req, ok := h.decodeAndValidate(w, body)
	if !ok {
		return
	}

	if !h.reconcileMessageID(w, r, req) {
		return
	}

	msg := buildMessage(req)

	admitStart := time.Now()
	err := h.admitMessage(r.Context(), msg, body)
	h.metrics.ObserveHandlerDuration(time.Since(admitStart))

	switch {
	case errors.Is(err, ErrAlreadyAdmitted):
		h.metrics.ObserveDuplicate()
		w.WriteHeader(http.StatusOK)
	case err != nil:
		h.metrics.ObserveEnqueueFailure()
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		h.metrics.ObserveAccepted()
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) admitMessage(ctx context.Context, msg *Message, body []byte) error {
	h.admissionLock.RLock()

	if h.closed {
		h.admissionLock.RUnlock()

		return errClosed
	}

	h.admissions.Add(1)

	closedCh := h.closedCh
	h.admissionLock.RUnlock()

	defer h.admissions.Done()

	select {
	case <-closedCh:
		return errClosed
	default:
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("admit webhook message: %w", err)
	}

	admitCtx, timeoutCancel := context.WithTimeout(ctx, h.options.admitTimeout)
	admitCtx, shutdownCancel := context.WithCancel(admitCtx)
	//nolint:contextcheck // AfterFunc는 하위 호출에 context를 전달하는 것이 아니라 handler 수명 context에 취소 콜백을 등록한다.
	stopShutdownCancel := context.AfterFunc(h.runCtx, shutdownCancel)

	defer func() {
		stopShutdownCancel()
		shutdownCancel()
		timeoutCancel()
	}()

	if err := h.admitter.AdmitMessage(admitCtx, msg, body); err != nil {
		return fmt.Errorf("admit webhook message: %w", err)
	}

	return nil
}

func (h *Handler) SignatureVersionDiagnostics() SignatureVersionDiagnostics {
	if h == nil {
		return SignatureVersionDiagnostics{}
	}

	return SignatureVersionDiagnostics{
		V3Validated:       h.signatureV3Validated.Load(),
		UnknownRejected:   h.signatureUnknown.Load(),
		MalformedRejected: h.signatureMalformed.Load(),
	}
}
