package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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

// MessageHandler processes incoming webhook messages.
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message)
}

// HandlerOptions configures the WebhookHandler.
type HandlerOptions struct {
	WorkerCount    int
	QueueSize      int
	EnqueueTimeout time.Duration
	HandlerTimeout time.Duration
	RequireHTTP2   bool
	DedupTTL       time.Duration
	DedupTimeout   time.Duration
	MaxBodyBytes   int64
}

// Handler is the webhook HTTP handler with stripe worker pool.
type Handler struct {
	token      string
	tokenBytes []byte
	handler    MessageHandler
	dedup      Deduplicator
	logger     *slog.Logger
	metrics    Metrics
	options    HandlerOptions
	baseCtxFn  func() context.Context

	queueLock sync.RWMutex
	closed    bool
	sched     *scheduler
}

type webhookTask struct {
	msg *Message
}

// HandlerOption mutates a Handler during construction.
type HandlerOption func(*Handler)

// NewHandler creates a new WebhookHandler and starts workers.
// Ctx is used as base context for worker message handling.
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
	}
	result.tokenBytes = []byte(result.token)

	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}

	result.options = normalizeHandlerOptions(result.options)
	result.sched = newScheduler(result.options.QueueSize)
	result.sched.start(result.options.WorkerCount, result.makeTaskRunner(result.baseContext()))

	return result
}

// WithMetrics sets the metrics implementation.
func WithMetrics(m Metrics) HandlerOption {
	return func(h *Handler) {
		if m != nil {
			h.metrics = m
		}
	}
}

// WithDeduplicator sets the deduplicator implementation.
func WithDeduplicator(d Deduplicator) HandlerOption {
	return func(h *Handler) {
		if d != nil {
			h.dedup = d
		}
	}
}

// WithWorkerCount sets the worker count.
func WithWorkerCount(n int) HandlerOption {
	return func(h *Handler) {
		h.options.WorkerCount = n
	}
}

// WithQueueSize sets the queue size.
func WithQueueSize(n int) HandlerOption {
	return func(h *Handler) {
		h.options.QueueSize = n
	}
}

// WithEnqueueTimeout sets the enqueue timeout.
func WithEnqueueTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.EnqueueTimeout = d
	}
}

// WithHandlerTimeout sets the handler timeout.
func WithHandlerTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.HandlerTimeout = d
	}
}

// WithRequireHTTP2 toggles HTTP/2 enforcement.
func WithRequireHTTP2(b bool) HandlerOption {
	return func(h *Handler) {
		h.options.RequireHTTP2 = b
	}
}

// WithDedupTTL sets the deduplication TTL.
func WithDedupTTL(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.DedupTTL = d
	}
}

// WithDedupTimeout sets the deduplication timeout.
func WithDedupTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		h.options.DedupTimeout = d
	}
}

// WithMaxBodyBytes sets the maximum allowed request body size.
func WithMaxBodyBytes(n int64) HandlerOption {
	return func(h *Handler) {
		h.options.MaxBodyBytes = n
	}
}

// WithAutoWorkerCount sets worker count to runtime.GOMAXPROCS(0) with a floor of 4.
func WithAutoWorkerCount() HandlerOption {
	return func(h *Handler) {
		n := runtime.GOMAXPROCS(0)
		if n < 4 {
			n = 4
		}
		h.options.WorkerCount = n
	}
}

// Close stops workers and waits for queued work to drain.
func (h *Handler) Close() error {
	h.queueLock.Lock()
	if h.closed {
		h.queueLock.Unlock()

		return nil
	}

	h.closed = true
	h.queueLock.Unlock()

	h.sched.close()

	return nil
}

// ServeHTTP handles Iris webhook requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.metrics.ObserveRequest()

	if !isPOST(r.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	if h.rejectProtocol(w, r) || h.rejectMissingToken(w) || h.rejectUnauthorized(w, r) {
		return
	}

	duplicate, handled := h.handleDedup(w, r)
	if handled {
		if duplicate {
			h.metrics.ObserveDuplicate()
		}

		return
	}

	if !isJSONContentType(r.Header.Get("Content-Type")) {
		w.WriteHeader(http.StatusUnsupportedMediaType)

		return
	}

	req, ok := h.decodeAndValidate(w, r)
	if !ok {
		return
	}

	msg := buildMessage(req)
	if err := h.enqueue(webhookTask{msg: msg}); err != nil {
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

	decoder := json.NewDecoder(body)

	var req WebhookRequest
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("decode webhook request: %w", err)
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, fmt.Errorf("ensure single JSON value: %w", err)
	}

	return &req, nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
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

func (h *Handler) enqueue(task webhookTask) error {
	h.queueLock.RLock()
	defer h.queueLock.RUnlock()

	if h.closed {
		return errClosed
	}

	select {
	case h.sched.incoming <- task:
		h.metrics.ObserveEnqueueWait(0)
		return nil
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
	case h.sched.incoming <- task:
		h.metrics.ObserveEnqueueWait(time.Since(start))
		return nil
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
	defer func() {
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
		defer cancel()
	}

	if h.handler != nil {
		h.handler.HandleMessage(ctx, task.msg)
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
		UserID:     req.UserID,
		Message:    req.Text,
		ChatID:     req.Room,
		Route:      req.Route,
		MessageID:  req.MessageID,
		ChatLogID:  req.ChatLogID,
		RoomType:   req.RoomType,
		RoomLinkID: req.RoomLinkID,
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

	return result
}

func validWebhookRequest(req *WebhookRequest) bool {
	return validRequiredMax(req.Text, 16000) &&
		validRequiredMax(req.Room, 256) &&
		validRequiredMax(req.UserID, 256) &&
		validOptionalMax(req.Sender, 256) &&
		validOptionalMax(req.Route, 256) &&
		validOptionalMax(req.MessageID, 256) &&
		validOptionalMax(req.ChatLogID, 256) &&
		validOptionalMax(req.RoomType, 256) &&
		validOptionalMax(req.RoomLinkID, 256) &&
		validOptionalMax(req.ThreadID, 256)
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
