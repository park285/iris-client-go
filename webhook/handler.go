package webhook

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/v3/internal/irishmac"
)

const (
	defaultAdmitTimeout = 30 * time.Second
	defaultNonceTimeout = 200 * time.Millisecond
	defaultMaxBodyBytes = 1 << 20
	// ReplayWindow(5m)는 Iris sender의 최대 전송 지평(약 15m)보다 짧고 인증 성공 요청은 응답이
	// 400/503이어도 nonce가 set-once로 소모되지만, Iris sender는 attempt마다 요청을 새로 서명한다
	// (iris-runtime/src/webhook: send_permitted_delivery → signing_for_delivery가 매 attempt
	// new_webhook_nonce()·fresh timestamp 생성). 이 전제가 깨지면 503 후 재전송이 duplicate
	// nonce 401로 죽어 ec35264의 유실 차단이 무효화되므로, Iris 서명 경로를 바꿀 때는 이 계약을 함께 봐야 한다.
	defaultReplayWindow  = 5 * time.Minute
	maxEventPayloadBytes = 256 << 10

	envWebhookToken = "IRIS_WEBHOOK_TOKEN" // #nosec G101 -- 자격증명이 아니라 환경 변수 이름이다.
)

var errClosed = errors.New("webhook handler closed")

var (
	ErrMessageAdmitterRequired = errors.New("webhook: message admitter is required")
	ErrNonceStoreRequired      = errors.New("webhook: explicit set-once nonce store is required")
)

// ErrAlreadyAdmitted는 MessageAdmitter가 이미 admit된 메시지의 재수신을 알리는 sentinel이다.
// Handler는 이를 실패가 아닌 중복 수리로 간주해 duplicate 계측(Metrics.ObserveDuplicate)을
// 관측하고 200 OK를 반환한다.
var ErrAlreadyAdmitted = errors.New("webhook: message already admitted")

// MessageAdmitter는 HTTP 200 전에 메시지를 durable store에 commit하는 계약이다.
// 구현은 이미 admit된 메시지를 감지하면 ErrAlreadyAdmitted(래핑 포함, errors.Is로 판정)를
// 반환할 수 있다. 그 외 모든 오류는 503으로 매핑되어 Iris sender의 재전송을 유도한다.
type MessageAdmitter interface {
	AdmitMessage(ctx context.Context, msg *Message) error
}

// SignatureVersionDiagnostics는 HMAC 검증 version의 고정 cardinality 누적값입니다.
type SignatureVersionDiagnostics struct {
	V3Validated       uint64 `json:"v3Validated"`
	UnknownRejected   uint64 `json:"unknownRejected"`
	MalformedRejected uint64 `json:"malformedRejected"`
}

// Handler는 HMAC 검증을 통과한 Iris webhook을 HTTP 200 전에 durable admission으로 넘기는 핸들러입니다.
type Handler struct {
	token         string
	replayWindow  time.Duration
	nonceStore    NonceStore
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

// NewDurableHandler는 옵션을 순서대로 한 번 적용한 뒤 인증·nonce 설정을 검증하고 durable admission
// 전용 handler를 만든다. Admitter가 nil이면 옵션 실행 전에 거절한다. WithWebhookToken이 비어 있으면
// 옵션 적용 뒤 IRIS_WEBHOOK_TOKEN을 읽는다. 메시지 실행(dispatch)은 소비자의 inbox가 소유한다.
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

	// 환경값은 옵션 실행 뒤에 읽어 기존 SDK의 설정 해석 순서를 보존한다.
	h.token = cmp.Or(strings.TrimSpace(h.token), strings.TrimSpace(os.Getenv(envWebhookToken)))
	if h.token == "" {
		return nil, errors.New("iris: webhook token is required (set IRIS_WEBHOOK_TOKEN or webhook.WithWebhookToken)")
	}

	if !isSetOnceNonceStore(h.nonceStore) {
		return nil, fmt.Errorf("iris: new durable webhook handler: %w", ErrNonceStoreRequired)
	}

	h.logger = resolveLogger(h.logger)
	h.options = normalizeHandlerOptions(h.options)
	h.webhookSigner = irishmac.NewSigner(h.token)

	// HTTP receive context는 decode/admission까지만 소유한다. 실행 context는 startup
	// snapshot의 값을 보존하되 shutdown이 시작될 때만 취소한다.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(cmp.Or(h.baseCtx, context.Background())))

	h.runCtx, h.runCancel = runCtx, runCancel //nolint:fatcontext // 생성 시 한 번만 파생하는 핸들러 수명 context다.

	return h, nil
}

// Close는 admission을 닫고 진행 중인 admission이 끝날 때까지 기다린다. 제한 없는 context로
// CloseContext를 부르는 것과 같다. 표준 io.Closer 관례를 따르는 표면이라 유지하며 폐기 대상이 아니다
// (DEC-20260926-stack-iris-client-go-compat-surface-retirement 제외 항목). 종료 시간 상한이 있으면
// CloseContext를 쓴다.
func (h *Handler) Close() error {
	return h.CloseContext(context.Background())
}

// CloseContext는 새 admission을 503으로 거절하고, grace context가 끝나면 진행 중인 admission
// context를 취소한다.
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

// ServeHTTP는 Iris webhook 요청을 처리합니다.
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
	err := h.admitMessage(r.Context(), msg)
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

// admitMessage는 요청 context와 AdmitTimeout, 핸들러 수명 context 중 먼저 끝나는 쪽으로 admission을 묶는다.
func (h *Handler) admitMessage(ctx context.Context, msg *Message) error {
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

	if err := h.admitter.AdmitMessage(admitCtx, msg); err != nil {
		return fmt.Errorf("admit webhook message: %w", err)
	}

	return nil
}

// SignatureVersionDiagnostics는 signature compare 성공과 version 거절 누적값을 반환합니다.
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
