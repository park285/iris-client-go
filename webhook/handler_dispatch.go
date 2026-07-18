package webhook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

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
