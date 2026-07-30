package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	dedupCommitAttempts   = 3
	dedupCommitRetryDelay = 20 * time.Millisecond
	dedupKeyHashHexLen    = 12
)

type dedupReservation struct {
	key      string
	token    string
	stateful bool
	// Reserve가 오류로 끝나 예약 성립 여부를 모르는 상태. 정리는 best-effort이고
	// ErrDedupReservationLost는 정상 결과다.
	orphaned bool
}

func (h *Handler) handleDedupKey(w http.ResponseWriter, r *http.Request, key string) (bool, dedupReservation) {
	key = DedupKey(key)
	if key == "" {
		return false, dedupReservation{}
	}

	start := time.Now()
	state, token, err := h.reserveDedupKey(r.Context(), key)
	h.metrics.ObserveDedupLatency(time.Since(start))
	if err != nil {
		h.logger.Warn("webhook dedup degraded", slog.Any("error", err), dedupKeyAttr(key))

		return false, h.orphanedReservation(key, token)
	}

	switch state {
	case DedupStatePending:
		h.dedupPendingRejected.Add(1)
		if h.dedupPendingObserver != nil {
			h.dedupPendingObserver.ObserveDedupPendingRejected()
		}
		w.WriteHeader(http.StatusServiceUnavailable)

		return true, dedupReservation{}
	case DedupStateCommitted:
		h.metrics.ObserveDuplicate()
		w.WriteHeader(http.StatusOK)

		return true, dedupReservation{}
	default:
		return false, dedupReservation{key: key, token: token, stateful: h.statefulDedup != nil}
	}
}

func (h *Handler) orphanedReservation(key, token string) dedupReservation {
	if h.statefulDedup == nil || token == "" {
		return dedupReservation{}
	}

	return dedupReservation{key: key, token: token, stateful: true, orphaned: true}
}

func (h *Handler) reserveDedupKey(ctx context.Context, key string) (DedupState, string, error) {
	if h.statefulDedup == nil {
		duplicate, err := h.isDuplicate(ctx, key)
		switch {
		case err != nil:
			return DedupStateReserved, "", err
		case duplicate:
			return DedupStateCommitted, "", nil
		default:
			return DedupStateReserved, "", nil
		}
	}

	dedupCtx, cancel := h.dedupContext(ctx)
	defer cancel()

	token, state, err := h.statefulDedup.Reserve(dedupCtx, key, h.options.DedupPendingTTL)
	if err != nil {
		return DedupStateReserved, token, err
	}
	if state < DedupStateReserved || state > DedupStateCommitted {
		return DedupStateReserved, token, fmt.Errorf("dedup reserve returned invalid state %d", state)
	}
	if state == DedupStateReserved && token == "" {
		return DedupStateReserved, "", errors.New("dedup reserve returned an empty owner token")
	}

	return state, token, nil
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

// enqueue 성공 후에만 호출되고 응답은 어떤 경우에도 200을 유지한다. 일시 오류를 bounded
// 재시도하는 것은 실패 방향을 503 거부가 아니라 200 흡수 쪽으로 기울이기 위해서다.
func (h *Handler) commitDedupReservation(ctx context.Context, reservation dedupReservation) {
	if !reservation.stateful || reservation.key == "" {
		return
	}

	commitCtx, cancel := h.dedupContext(context.WithoutCancel(ctx))
	defer cancel()

	err := h.commitWithRetry(commitCtx, reservation)
	switch {
	case err == nil:
	case errors.Is(err, ErrDedupReservationLost) && reservation.orphaned:
		h.logger.Debug(
			"webhook dedup commit found no reservation for a degraded reserve; nothing to reclaim",
			slog.Any("error", err),
			dedupKeyAttr(reservation.key),
		)
	case errors.Is(err, ErrDedupReservationLost):
		h.logger.Warn(
			"webhook dedup commit lost its reservation; the key is no longer owned by this request, so a retransmission of this already-processed message will be processed again",
			slog.Any("error", err),
			dedupKeyAttr(reservation.key),
		)
	case reservation.orphaned:
		h.logger.Warn(
			"webhook dedup commit failed for a degraded reserve; whether a reservation exists is unknown, so a retransmission of this already-processed message is either processed again or rejected with 503 until the reservation expires",
			slog.Any("error", err),
			dedupKeyAttr(reservation.key),
		)
	default:
		h.logger.Warn(
			"webhook dedup commit failed; the reservation stays pending, so retransmissions of this already-processed message are rejected with 503 until it expires",
			slog.Any("error", err),
			dedupKeyAttr(reservation.key),
		)
	}
}

func (h *Handler) commitWithRetry(ctx context.Context, reservation dedupReservation) error {
	var err error
	for attempt := range dedupCommitAttempts {
		if attempt > 0 && !sleepContext(ctx, dedupCommitRetryDelay) {
			return err
		}

		err = h.statefulDedup.Commit(ctx, reservation.key, reservation.token, h.options.DedupTTL)
		if err == nil {
			return nil
		}
		// 다른 owner가 쥔 키를 강제로 덮어쓰지 않는다.
		if errors.Is(err, ErrDedupReservationLost) {
			return err
		}
	}

	return err
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *Handler) releaseDedupKey(ctx context.Context, reservation dedupReservation) {
	if reservation.key == "" || h.dedup == nil {
		return
	}

	// enqueue 실패 원인이 request context 취소 자체일 수 있으므로 취소를 끊고
	// DedupTimeout으로만 상한을 둔다.
	releaseCtx, cancel := h.dedupContext(context.WithoutCancel(ctx))
	defer cancel()

	err := h.releaseReservation(releaseCtx, reservation)
	switch {
	case err == nil:
	case errors.Is(err, ErrDedupReservationLost) && reservation.orphaned:
		h.logger.Debug(
			"webhook dedup release found no reservation for a degraded reserve; nothing to reclaim",
			slog.Any("error", err),
			dedupKeyAttr(reservation.key),
		)
	default:
		h.logger.Warn("webhook dedup release failed", slog.Any("error", err), dedupKeyAttr(reservation.key))
	}
}

func (h *Handler) releaseReservation(ctx context.Context, reservation dedupReservation) error {
	if reservation.stateful {
		return h.statefulDedup.ReleaseReservation(ctx, reservation.key, reservation.token)
	}

	releaser, ok := h.dedup.(DedupReleaser)
	if !ok {
		return nil
	}

	return releaser.Release(ctx, reservation.key)
}

func (h *Handler) dedupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, h.options.DedupTimeout)
}

func dedupKeyAttr(key string) slog.Attr {
	sum := sha256.Sum256([]byte(key))

	return slog.String("dedupKeyHash", hex.EncodeToString(sum[:])[:dedupKeyHashHexLen])
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
