package webhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/park285/iris-client-go/internal/irishmac"
	"github.com/park285/iris-client-go/internal/jsonx"
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

type HandlerOptions struct {
	WorkerCount    int
	QueueSize      int
	EnqueueTimeout time.Duration
	AdmitTimeout   time.Duration
	HandlerTimeout time.Duration
	OrderingMode   OrderingMode
	DedupTTL       time.Duration
	DedupTimeout   time.Duration
	DedupMode      DedupMode
	MaxBodyBytes   int64
}

type OrderingMode int

const (
	OrderingModeKey OrderingMode = iota
	OrderingModeNone
)

type DedupMode int

const (
	DedupModeBeforeDecode DedupMode = iota
	DedupModeAfterDecode
)

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

type HandlerOption func(*Handler)

// NewHandler는 워커를 즉시 시작합니다. ctx는 워커 메시지 처리의 기본 context로 사용됩니다.
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

// WithDedupMode is retained for source compatibility. Deduplication always
// happens after authentication, body decoding, request validation, and message
// identity reconciliation. DedupModeBeforeDecode is no longer honored because a
// side-effecting backend could otherwise reserve an authenticated message ID for
// a request that is later rejected.
func WithDedupMode(mode DedupMode) HandlerOption {
	return func(h *Handler) {
		_ = mode
		h.options.DedupMode = DedupModeAfterDecode
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

func (h *Handler) acceptTransport(w http.ResponseWriter, r *http.Request) bool {
	if !isPOST(r.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	if h.rejectMissingToken(w) {
		return false
	}
	if !h.rejectUnauthorized(w, r) {
		return false
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (h *Handler) rejectMissingToken(w http.ResponseWriter) bool {
	if h.token != "" || h.webhookSecret != "" {
		return false
	}

	w.WriteHeader(http.StatusInternalServerError)

	return true
}

func (h *Handler) rejectUnauthorized(w http.ResponseWriter, r *http.Request) bool {
	if hasSignatureHeaders(r.Header) {
		body, ok := h.bufferBodyForHMAC(w, r)
		if !ok {
			return false
		}
		if h.authorizeHMAC(r, body) {
			return true
		}
		h.metrics.ObserveUnauthorized()
		w.WriteHeader(http.StatusUnauthorized)

		return false
	}

	h.metrics.ObserveUnauthorized()
	w.WriteHeader(http.StatusUnauthorized)

	return false
}

func (h *Handler) bufferBodyForHMAC(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body := http.MaxBytesReader(w, r.Body, h.options.MaxBodyBytes)
	raw, err := io.ReadAll(body)
	closeErr := body.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(statusForDecodeError(err))

		return nil, false
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, true
}

func (h *Handler) authorizeHMAC(r *http.Request, body []byte) bool {
	if !hasWebhookSignatureVersionV2(r.Header) {
		return false
	}
	timestamp, nonce, signature, bodySHA256, ok := signatureHeaderValues(r.Header)
	if !ok || !timestampWithinReplayWindow(timestamp, h.replayWindow, time.Now()) {
		return false
	}

	gotBodySHA256 := irishmac.SHA256HexBytes(body)
	if !constantTimeEqualString(bodySHA256, gotBodySHA256) {
		return false
	}

	target, err := irishmac.CanonicalTarget(r.URL.RequestURI())
	if err != nil {
		return false
	}
	messageID, present, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !present {
		return false
	}
	canonical := canonicalWebhookRequestV2(r.Method, target, timestamp, nonce, messageID, gotBodySHA256)
	expected := h.webhookSigner.Sign(canonical)
	if !constantTimeEqualString(signature, expected) {
		return false
	}

	return !h.isReplay(r.Context(), r.Method, target, timestamp, nonce)
}

func canonicalWebhookRequestV2(method, target, timestamp, nonce, messageID, bodySHA256 string) string {
	return strings.Join([]string{
		SignatureVersionV2,
		strings.ToUpper(method),
		target,
		timestamp,
		nonce,
		messageID,
		strings.ToLower(bodySHA256),
	}, "\n")
}

func hasSignatureHeaders(header http.Header) bool {
	return header.Get(HeaderIrisTimestamp) != "" ||
		header.Get(HeaderIrisNonce) != "" ||
		header.Get(HeaderIrisSignature) != "" ||
		header.Get(HeaderIrisBodySHA256) != "" ||
		header.Get(HeaderIrisSignatureVersion) != ""
}

func hasWebhookSignatureVersionV2(header http.Header) bool {
	values := header.Values(HeaderIrisSignatureVersion)
	return len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), SignatureVersionV2)
}

func signatureHeaderValues(header http.Header) (string, string, string, string, bool) {
	timestamp := strings.TrimSpace(header.Get(HeaderIrisTimestamp))
	nonce := strings.TrimSpace(header.Get(HeaderIrisNonce))
	signature := strings.TrimSpace(header.Get(HeaderIrisSignature))
	bodySHA256 := strings.TrimSpace(header.Get(HeaderIrisBodySHA256))
	return timestamp, nonce, signature, bodySHA256, timestamp != "" && nonce != "" && signature != "" && bodySHA256 != ""
}

func timestampWithinReplayWindow(timestamp string, window time.Duration, now time.Time) bool {
	timestampMs, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.UnixMilli(timestampMs))
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

func (h *Handler) isReplay(ctx context.Context, method, target, timestamp, nonce string) bool {
	if h.nonceCache == nil {
		return true
	}
	key := strings.Join([]string{strings.ToUpper(method), target, timestamp, nonce}, "\n")
	duplicate, err := h.isNonceDuplicate(ctx, key)
	if err != nil {
		h.logger.Warn("webhook hmac nonce check failed", slog.Any("error", err))

		return true
	}
	return duplicate
}

func (h *Handler) isNonceDuplicate(ctx context.Context, key string) (bool, error) {
	dedupCtx := ctx
	cancel := func() {}
	if h.options.DedupTimeout > 0 {
		dedupCtx, cancel = context.WithTimeout(ctx, h.options.DedupTimeout)
	}
	defer cancel()
	return h.nonceCache.IsDuplicate(dedupCtx, key, h.nonceReplayTTL())
}

// timestamp를 미래 방향으로 window까지(now+window) 수용하므로, 서명자 시계가 앞선 nonce가
// 만료 후 (now+window, ts+window] 구간에서 재사용되지 않으려면 최초 수신(now-window)부터
// 최종 수용(ts+window)까지 최대 2*window를 덮어야 한다.
func (h *Handler) nonceReplayTTL() time.Duration {
	return 2 * h.replayWindow
}

func constantTimeEqualString(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request) (*WebhookRequest, bool) {
	start := time.Now()
	req, err := decodeWebhookRequest(w, r, h.options.MaxBodyBytes)
	h.metrics.ObserveDecodeLatency(time.Since(start))
	status := 0
	if err != nil {
		h.logger.Warn("webhook decode failed", slog.Any("error", err))
		status = statusForDecodeError(err)
	} else if !validWebhookRequest(req) {
		status = http.StatusBadRequest
	}
	if status != 0 {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(status)

		return nil, false
	}

	return req, true
}

func canonicalDedupID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	return req.MessageID
}

func (h *Handler) reconcileMessageID(w http.ResponseWriter, r *http.Request, req *WebhookRequest) bool {
	bodyID, valid := normalizeMessageID(req.MessageID)
	if !valid {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return false
	}
	headerID, headerPresent, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !headerPresent || (bodyID != "" && bodyID != headerID) {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return false
	}

	if bodyID == "" {
		req.MessageID = headerID
	} else {
		req.MessageID = bodyID
	}

	return true
}

func normalizedMessageIDHeader(header http.Header) (string, bool, bool) {
	values := header.Values(HeaderIrisMessageID)
	if len(values) > 1 {
		return "", false, false
	}
	if len(values) == 0 {
		return "", false, true
	}

	messageID, valid := normalizeMessageID(values[0])

	return messageID, messageID != "", valid
}

func normalizeMessageID(raw string) (string, bool) {
	messageID := strings.TrimSpace(raw)
	if messageID == "" {
		return "", true
	}
	if len(messageID) > maxMessageIDBytes {
		return "", false
	}
	for i := range len(messageID) {
		if validMessageIDByte(messageID[i]) {
			continue
		}

		return "", false
	}

	return messageID, true
}

func validMessageIDByte(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		strings.ContainsRune("-_.:/", rune(character))
}

func (h *Handler) handleDedupKey(w http.ResponseWriter, r *http.Request, key string) (bool, bool, string) {
	key = DedupKey(key)
	if key == "" {
		return false, false, ""
	}

	start := time.Now()
	duplicate, err := h.isDuplicate(r.Context(), key)
	h.metrics.ObserveDedupLatency(time.Since(start))
	if err != nil {
		h.logger.Warn("webhook dedup degraded", slog.Any("error", err), slog.String("key", key))

		return false, false, ""
	}

	if !duplicate {
		return false, false, key
	}

	w.WriteHeader(http.StatusOK)

	return true, true, ""
}

func (h *Handler) isDuplicate(ctx context.Context, key string) (bool, error) {
	if h.dedup == nil {
		return false, nil
	}

	dedupCtx := ctx
	cancel := func() {}

	if h.options.DedupTimeout > 0 {
		dedupCtx, cancel = context.WithTimeout(ctx, h.options.DedupTimeout)
	}

	defer cancel()

	duplicate, err := h.dedup.IsDuplicate(dedupCtx, key, h.options.DedupTTL)
	if err != nil {
		return false, fmt.Errorf("dedup check: %w", err)
	}

	return duplicate, nil
}

func (h *Handler) releaseDedupKey(ctx context.Context, key string) {
	if key == "" || h.dedup == nil {
		return
	}
	releaser, ok := h.dedup.(DedupReleaser)
	if !ok {
		return
	}

	// enqueue 실패 원인이 request context 취소 자체일 수 있으므로 취소를 끊고
	// DedupTimeout으로만 상한을 둔다.
	releaseCtx := context.WithoutCancel(ctx)
	cancel := func() {}
	if h.options.DedupTimeout > 0 {
		releaseCtx, cancel = context.WithTimeout(releaseCtx, h.options.DedupTimeout)
	}
	defer cancel()

	if err := releaser.Release(releaseCtx, key); err != nil {
		h.logger.Warn("webhook dedup release failed", slog.Any("error", err), slog.String("key", key))
	}
}

func decodeWebhookRequest(
	w http.ResponseWriter,
	r *http.Request,
	maxBodyBytes int64,
) (*WebhookRequest, error) {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)

	defer func() {
		_ = body.Close() //nolint:errcheck // 디코딩 후 request body를 닫는 것은 best-effort다.
	}()

	decoder := jsonx.NewDecoder(body)

	var req WebhookRequest
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("decode webhook request: %w", err)
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, fmt.Errorf("ensure single JSON value: %w", err)
	}

	return &req, nil
}

func ensureSingleJSONValue(decoder jsonx.Decoder) error {
	var extra struct{}
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("webhook request contains multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode trailing JSON value: %w", err)
	}

	return nil
}

func statusForDecodeError(err error) int {
	if isBodyTooLarge(err) {
		return http.StatusRequestEntityTooLarge
	}

	return http.StatusBadRequest
}

func isBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError

	return errors.As(err, &maxBytesErr)
}

func (h *Handler) enqueueTask(ctx context.Context, task webhookTask) error {
	if ctx == nil {
		ctx = context.Background()
	}

	h.queueLock.RLock()
	if h.closed {
		h.queueLock.RUnlock()
		return errClosed
	}

	incoming := h.sched.incomingFor(task)
	closedCh := h.closedCh
	h.enqueueWG.Add(1)
	h.queueLock.RUnlock()
	defer h.enqueueWG.Done()

	select {
	case <-closedCh:
		return errClosed
	default:
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case incoming <- task:
		h.metrics.ObserveEnqueueWait(0)
		h.metrics.ObserveQueueDepth(int(h.sched.depth.Load()))
		return nil
	case <-closedCh:
		return errClosed
	default:
	}

	start := time.Now()
	timer := time.NewTimer(h.options.EnqueueTimeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case incoming <- task:
		h.metrics.ObserveEnqueueWait(time.Since(start))
		h.metrics.ObserveQueueDepth(int(h.sched.depth.Load()))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-closedCh:
		return errClosed
	case <-timer.C:
		return errQueueFull
	}
}

func (h *Handler) admitMessage(ctx context.Context, msg *Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	h.queueLock.RLock()
	if h.closed {
		h.queueLock.RUnlock()

		return errClosed
	}
	h.enqueueWG.Add(1)
	closedCh := h.closedCh
	h.queueLock.RUnlock()
	defer h.enqueueWG.Done()
	select {
	case <-closedCh:
		return errClosed
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	admitCtx := ctx
	timeoutCancel := func() {}
	if h.options.AdmitTimeout > 0 {
		admitCtx, timeoutCancel = context.WithTimeout(ctx, h.options.AdmitTimeout)
	}
	admitCtx, shutdownCancel := context.WithCancel(admitCtx)
	stopShutdownCancel := context.AfterFunc(h.runCtx, shutdownCancel)
	defer func() {
		stopShutdownCancel()
		shutdownCancel()
		timeoutCancel()
	}()

	return h.admitter.AdmitMessage(admitCtx, msg)
}

func (h *Handler) makeTaskRunner(baseCtx context.Context) taskRunner {
	return func(_ int, task webhookTask) {
		h.runTask(baseCtx, task)
	}
}

func (h *Handler) runTask(baseCtx context.Context, task webhookTask) {
	start := time.Now()
	h.activeTasks.Add(1)
	defer func() {
		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()
	defer func() {
		h.activeTasks.Add(-1)
		h.metrics.ObserveHandlerDuration(time.Since(start))
	}()

	ctx := baseCtx
	if ctx == nil {
		ctx = context.Background()
	}

	if h.options.HandlerTimeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, h.options.HandlerTimeout)
		defer func() {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				h.handlerTimeouts.Add(1)
			}
			cancel()
		}()
	}

	if ctx.Err() == nil && h.handler != nil {
		h.handler.HandleMessage(ctx, task.msg)
	}
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

func stripeKey(msg *Message) string {
	if msg == nil {
		return ""
	}

	room := strings.TrimSpace(msg.Room)

	threadID := messageThreadID(msg)
	if room == "" || threadID == "" {
		return room
	}

	return room + ":" + threadID
}

func messageThreadID(msg *Message) string {
	if msg == nil || msg.JSON == nil || msg.JSON.ThreadID == nil {
		return ""
	}

	return strings.TrimSpace(*msg.JSON.ThreadID)
}

func buildMessage(req *WebhookRequest) *Message {
	trimmed := normalizeWebhookRequest(req)
	msg := &Message{
		Msg:  trimmed.Text,
		Room: trimmed.Room,
		JSON: buildMessageJSON(trimmed),
	}

	if trimmed.Sender != "" {
		sender := trimmed.Sender

		msg.Sender = &sender
	}

	return msg
}

func buildMessageJSON(req WebhookRequest) *MessageJSON {
	result := &MessageJSON{
		UserID:             req.UserID,
		Message:            req.Text,
		ChatID:             req.Room,
		Type:               req.Type,
		Route:              req.Route,
		MessageID:          req.MessageID,
		ChatLogID:          req.ChatLogID,
		RoomType:           req.RoomType,
		RoomLinkID:         req.RoomLinkID,
		RawSourceLogID:     req.RawSourceLogID,
		SourceGenerationID: req.SourceGenerationID,
		SourceAccountID:    req.SourceAccountID,
		IsMine:             req.IsMine,
		Origin:             req.Origin,
		Attachment:         req.Attachment,
		Mentions:           cloneWebhookMentions(req.Mentions),
		EventPayload:       req.EventPayload,
	}

	if req.SourceLogID != 0 {
		sourceLogID := req.SourceLogID

		result.SourceLogID = &sourceLogID
	}

	if threadID := ResolveThreadID(&req); threadID != "" {
		result.ThreadID = &threadID
	}

	if req.ThreadScope != nil {
		scope := *req.ThreadScope

		result.ThreadScope = &scope
	}

	return result
}

func normalizeWebhookRequest(req *WebhookRequest) WebhookRequest {
	if req == nil {
		return WebhookRequest{}
	}

	result := *req

	result.Route = strings.TrimSpace(result.Route)
	result.MessageID = strings.TrimSpace(result.MessageID)
	result.SourceAccountID = strings.TrimSpace(result.SourceAccountID)
	result.Sender = strings.TrimSpace(result.Sender)
	result.ChatLogID = strings.TrimSpace(result.ChatLogID)
	result.RoomType = strings.TrimSpace(result.RoomType)
	result.RoomLinkID = strings.TrimSpace(result.RoomLinkID)
	result.ThreadID = strings.TrimSpace(result.ThreadID)
	result.Type = strings.TrimSpace(result.Type)
	result.Origin = strings.TrimSpace(result.Origin)
	result.Mentions = cloneWebhookMentions(result.Mentions)

	return result
}

func cloneWebhookMentions(mentions []WebhookMention) []WebhookMention {
	if len(mentions) == 0 {
		return nil
	}

	out := make([]WebhookMention, 0, len(mentions))
	for _, mention := range mentions {
		mention.UserID = strings.TrimSpace(mention.UserID)
		mention.Nickname = strings.TrimSpace(mention.Nickname)
		mention.At = append([]int(nil), mention.At...)
		out = append(out, mention)
	}

	return out
}

func validWebhookRequest(req *WebhookRequest) bool {
	return validWebhookText(req) &&
		validRequiredMax(req.Room, 256) &&
		validRequiredMax(req.UserID, 256) &&
		validOptionalMax(req.Sender, 256) &&
		validOptionalMax(req.Route, 256) &&
		validOptionalMessageID(req.MessageID) &&
		validOptionalMax(req.SourceAccountID, 256) &&
		validOptionalMax(req.ChatLogID, 256) &&
		validOptionalMax(req.RoomType, 256) &&
		validOptionalMax(req.RoomLinkID, 256) &&
		validOptionalMax(req.ThreadID, 256) &&
		validOptionalMax(req.Type, 256) &&
		validOptionalMax(req.Origin, 64) &&
		(req.Attachment == "" || utf8.RuneCountInString(req.Attachment) <= 65536) &&
		len(req.EventPayload) <= maxEventPayloadBytes
}

func validWebhookText(req *WebhookRequest) bool {
	if utf8.RuneCountInString(req.Text) > 16000 {
		return false
	}

	if strings.TrimSpace(req.Text) != "" {
		return true
	}

	return strings.TrimSpace(req.Type) != "" && strings.TrimSpace(string(req.EventPayload)) != ""
}

func validRequiredMax(value string, limit int) bool {
	if utf8.RuneCountInString(value) > limit {
		return false
	}

	return strings.TrimSpace(value) != ""
}

func validOptionalMax(value string, limit int) bool {
	return utf8.RuneCountInString(value) <= limit
}

func validOptionalMessageID(value string) bool {
	_, valid := normalizeMessageID(value)

	return valid
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, "application/json")
}

func isPOST(method string) bool {
	return method == http.MethodPost
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
		DedupMode:      DedupModeAfterDecode,
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

	// Pre-decode deduplication is intentionally disabled. A SET-NX style
	// backend must not retain message identity until the body and identity have
	// both passed validation.
	opts.DedupMode = DedupModeAfterDecode

	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = defaultMaxBodyBytes
	}

	return opts
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
