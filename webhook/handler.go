package webhook

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/park285/iris-client-go/internal/jsonx"
)

const (
	defaultWorkerCount    = 16
	defaultQueueSize      = 1000
	defaultEnqueueTimeout = 50 * time.Millisecond
	defaultHandlerTimeout = 30 * time.Second
	defaultDedupTimeout   = 200 * time.Millisecond
	defaultMaxBodyBytes   = 1 << 20
)

var (
	errQueueFull = errors.New("webhook queue full")
	errClosed    = errors.New("webhook handler closed")
)

// MessageHandler는 수신된 webhook 메시지를 처리하는 인터페이스입니다.
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message)
}

type TaskPool interface {
	SubmitWait(task func()) bool
}

type HandlerOptions struct {
	WorkerCount    int
	QueueSize      int
	EnqueueTimeout time.Duration
	HandlerTimeout time.Duration
	OrderingMode   OrderingMode
	RequireHTTP2   bool
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
	token      string
	tokenBytes []byte
	handler    MessageHandler
	dedup      Deduplicator
	logger     *slog.Logger
	metrics    Metrics
	options    HandlerOptions
	baseCtxFn  func() context.Context

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
		token:     strings.TrimSpace(token),
		handler:   handler,
		dedup:     NoopDeduplicator{},
		logger:    resolveLogger(logger),
		metrics:   NoopMetrics{},
		options:   defaultHandlerOptions(),
		baseCtxFn: contextSource(ctx),
		closedCh:  make(chan struct{}),
	}
	result.tokenBytes = []byte(result.token)

	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}

	result.options = normalizeHandlerOptions(result.options)
	if result.taskPool == nil {
		result.taskPool = newInternalPool(result.options.WorkerCount, result.options.QueueSize)
		result.ownsPool = true
	}
	result.sched = newScheduler(result.options.QueueSize, result.taskPool, result.options.OrderingMode)
	result.sched.start(result.options.WorkerCount, result.makeTaskRunner(result.baseContext()))

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

func WithRequireHTTP2(b bool) HandlerOption {
	return func(h *Handler) {
		h.options.RequireHTTP2 = b
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

func WithDedupMode(mode DedupMode) HandlerOption {
	return func(h *Handler) {
		h.options.DedupMode = mode
	}
}

func WithMaxBodyBytes(n int64) HandlerOption {
	return func(h *Handler) {
		h.options.MaxBodyBytes = n
	}
}

// WithAutoWorkerCount는 워커 수를 runtime.GOMAXPROCS(0) 값으로 설정하며 최솟값은 4입니다.
func WithAutoWorkerCount() HandlerOption {
	return func(h *Handler) {
		n := max(runtime.GOMAXPROCS(0), 4)
		h.options.WorkerCount = n
	}
}

// Close는 워커를 중지하고 대기열의 작업이 모두 처리될 때까지 기다립니다.
func (h *Handler) Close() error {
	h.queueLock.Lock()
	if h.closed {
		h.queueLock.Unlock()

		return nil
	}

	h.closed = true
	close(h.closedCh)
	h.queueLock.Unlock()

	h.enqueueWG.Wait()
	h.sched.close()
	if h.ownsPool {
		if stopper, ok := h.taskPool.(interface{ StopAndWait() }); ok {
			stopper.StopAndWait()
		}
	}

	return nil
}

// ServeHTTP는 Iris webhook 요청을 처리합니다.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.metrics.ObserveRequest()

	if !isPOST(r.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	if h.rejectProtocol(w, r) || h.rejectMissingToken(w) || h.rejectUnauthorized(w, r) {
		return
	}

	if !isJSONContentType(r.Header.Get("Content-Type")) {
		w.WriteHeader(http.StatusUnsupportedMediaType)

		return
	}

	if h.options.DedupMode == DedupModeBeforeDecode {
		duplicate, handled := h.handleDedup(w, r)
		if handled {
			if duplicate {
				h.metrics.ObserveDuplicate()
			}

			return
		}
	}

	req, ok := h.decodeAndValidate(w, r)
	if !ok {
		return
	}

	if h.options.DedupMode == DedupModeAfterDecode {
		duplicate, handled := h.handleDedup(w, r)
		if handled {
			if duplicate {
				h.metrics.ObserveDuplicate()
			}

			return
		}
	}

	msg := buildMessage(req)
	if err := h.enqueueTask(r.Context(), webhookTask{msg: msg}); err != nil {
		h.enqueueRejected.Add(1)
		if errors.Is(err, errQueueFull) {
			h.queueFull.Add(1)
		}
		h.metrics.ObserveEnqueueFailure()
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	h.metrics.ObserveAccepted()
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) rejectProtocol(w http.ResponseWriter, r *http.Request) bool {
	if !h.options.RequireHTTP2 || r.ProtoMajor == 2 {
		return false
	}

	w.WriteHeader(http.StatusHTTPVersionNotSupported)

	return true
}

func (h *Handler) rejectMissingToken(w http.ResponseWriter) bool {
	if h.token != "" {
		return false
	}

	w.WriteHeader(http.StatusInternalServerError)

	return true
}

func (h *Handler) rejectUnauthorized(w http.ResponseWriter, r *http.Request) bool {
	provided := r.Header.Get(HeaderIrisToken)
	if subtle.ConstantTimeCompare([]byte(provided), h.tokenBytes) == 1 {
		return false
	}

	h.metrics.ObserveUnauthorized()
	w.WriteHeader(http.StatusUnauthorized)

	return true
}

func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request) (*WebhookRequest, bool) {
	start := time.Now()
	req, err := decodeWebhookRequest(w, r, h.options.MaxBodyBytes)
	h.metrics.ObserveDecodeLatency(time.Since(start))
	if err != nil {
		h.logger.Warn("webhook decode failed", slog.Any("error", err))
		h.metrics.ObserveBadRequest()
		w.WriteHeader(statusForDecodeError(err))

		return nil, false
	}

	if !validWebhookRequest(req) {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return nil, false
	}

	return req, true
}

func (h *Handler) handleDedup(w http.ResponseWriter, r *http.Request) (bool, bool) {
	key := DedupKey(r.Header.Get(HeaderIrisMessageID))
	if key == "" {
		return false, false
	}

	start := time.Now()
	duplicate, err := h.isDuplicate(r.Context(), key)
	h.metrics.ObserveDedupLatency(time.Since(start))
	if err != nil {
		h.logger.Warn("webhook dedup degraded", slog.Any("error", err), slog.String("key", key))

		return false, false
	}

	if !duplicate {
		return false, false
	}

	w.WriteHeader(http.StatusOK)

	return true, true
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

func decodeWebhookRequest(
	w http.ResponseWriter,
	r *http.Request,
	maxBodyBytes int64,
) (*WebhookRequest, error) {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)

	defer func() {
		_ = body.Close() //nolint:errcheck // Closing request body after decoding is best-effort.
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

func (h *Handler) makeTaskRunner(baseCtx context.Context) taskRunner {
	return func(index int, task webhookTask) {
		h.runTask(baseCtx, index, task)
	}
}

func (h *Handler) runTask(baseCtx context.Context, index int, task webhookTask) {
	start := time.Now()
	h.activeTasks.Add(1)
	defer func() {
		h.activeTasks.Add(-1)
		h.metrics.ObserveHandlerDuration(time.Since(start))
		if recovered := recover(); recovered != nil {
			h.logger.Error("webhook worker panic recovered", slog.Any("panic", recovered), slog.Int("worker", index))
		}
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

	if h.handler != nil {
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
		UserID:       req.UserID,
		Message:      req.Text,
		ChatID:       req.Room,
		Type:         req.Type,
		Route:        req.Route,
		MessageID:    req.MessageID,
		ChatLogID:    req.ChatLogID,
		RoomType:     req.RoomType,
		RoomLinkID:   req.RoomLinkID,
		IsMine:       req.IsMine,
		Origin:       req.Origin,
		Attachment:   req.Attachment,
		Mentions:     cloneWebhookMentions(req.Mentions),
		EventPayload: req.EventPayload,
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
		validOptionalMax(req.MessageID, 256) &&
		validOptionalMax(req.ChatLogID, 256) &&
		validOptionalMax(req.RoomType, 256) &&
		validOptionalMax(req.RoomLinkID, 256) &&
		validOptionalMax(req.ThreadID, 256) &&
		validOptionalMax(req.Type, 256) &&
		validOptionalMax(req.Origin, 64) &&
		(req.Attachment == "" || utf8.RuneCountInString(req.Attachment) <= 65536)
}

func validWebhookText(req *WebhookRequest) bool {
	text := strings.TrimSpace(req.Text)
	if text != "" {
		return utf8.RuneCountInString(text) <= 16000
	}

	return strings.TrimSpace(req.Type) != "" && strings.TrimSpace(string(req.EventPayload)) != ""
}

func validRequiredMax(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}

	return utf8.RuneCountInString(trimmed) <= limit
}

func validOptionalMax(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}

	return utf8.RuneCountInString(trimmed) <= limit
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

	if opts.DedupMode != DedupModeBeforeDecode && opts.DedupMode != DedupModeAfterDecode {
		opts.DedupMode = DedupModeBeforeDecode
	}

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
