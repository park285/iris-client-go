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
	key   string
	token string
}

func (h *Handler) handleDedupKey(w http.ResponseWriter, r *http.Request, key string) (bool, dedupReservation) {
	if h.messageDeduplicator == nil {
		return false, dedupReservation{}
	}

	key = DedupKey(key)
	if key == "" {
		return false, dedupReservation{}
	}

	start := time.Now()
	state, token, err := h.reserveDedupKey(r.Context(), key)
	h.metrics.ObserveDedupLatency(time.Since(start))
	if err != nil {
		h.releaseFailedReservation(r.Context(), dedupReservation{key: key, token: token})
		h.logger.Warn("webhook message dedup reserve failed; request rejected before dispatch", slog.Any("error", err), dedupKeyAttr(key))
		w.WriteHeader(http.StatusServiceUnavailable)

		return true, dedupReservation{}
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
		return false, dedupReservation{key: key, token: token}
	}
}

func (h *Handler) reserveDedupKey(ctx context.Context, key string) (DedupState, string, error) {
	if h.messageDeduplicator == nil {
		return DedupStateReserved, "", nil
	}

	dedupCtx, cancel := h.dedupContext(ctx)
	defer cancel()

	token, state, err := h.messageDeduplicator.Reserve(dedupCtx, key, h.dedupPendingTTL)
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

// enqueue 성공 후에만 호출되고 응답은 어떤 경우에도 200을 유지한다. 일시 오류를 bounded
// 재시도하는 것은 실패 방향을 503 거부가 아니라 200 흡수 쪽으로 기울이기 위해서다.
func (h *Handler) commitDedupReservation(ctx context.Context, reservation dedupReservation) {
	if reservation.key == "" {
		return
	}

	commitCtx, cancel := h.dedupContext(context.WithoutCancel(ctx))
	defer cancel()

	err := h.commitWithRetry(commitCtx, reservation)
	switch {
	case err == nil:
	case errors.Is(err, ErrDedupReservationLost):
		h.logger.Warn(
			"webhook dedup commit lost its reservation; the key is no longer owned by this request, so a retransmission of this already-processed message will be processed again",
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

		err = h.messageDeduplicator.Commit(ctx, reservation.key, reservation.token, h.options.DedupTTL)
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
	if reservation.key == "" || h.messageDeduplicator == nil {
		return
	}

	// enqueue 실패 원인이 request context 취소 자체일 수 있으므로 취소를 끊고
	// DedupTimeout으로만 상한을 둔다.
	releaseCtx, cancel := h.dedupContext(context.WithoutCancel(ctx))
	defer cancel()

	err := h.releaseReservation(releaseCtx, reservation)
	if err != nil {
		h.logger.Warn("webhook dedup release failed", slog.Any("error", err), dedupKeyAttr(reservation.key))
	}
}

func (h *Handler) releaseReservation(ctx context.Context, reservation dedupReservation) error {
	return h.messageDeduplicator.ReleaseReservation(ctx, reservation.key, reservation.token)
}

func (h *Handler) releaseFailedReservation(ctx context.Context, reservation dedupReservation) {
	if reservation.key == "" || reservation.token == "" || h.messageDeduplicator == nil {
		return
	}

	releaseCtx, cancel := h.dedupContext(context.WithoutCancel(ctx))
	defer cancel()

	err := h.releaseReservation(releaseCtx, reservation)
	if err != nil && !errors.Is(err, ErrDedupReservationLost) {
		h.logger.Warn("webhook message dedup reserve cleanup failed; pending TTL owns terminal cleanup", slog.Any("error", err), dedupKeyAttr(reservation.key))
	}
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

// StripeKey는 scheduler 경로의 직렬화 키를 반환한다. Room과 JSON.ThreadID는 각각 공백을
// 제거해 평가하며, 둘 다 비어 있지 않을 때만 "Room:ThreadID" 키가 되고 ThreadID가 없거나
// 공백뿐이면 Room 단독 키로 내려간다. OrderingModeKey에서 같은 키의 메시지는 도착
// 순서대로 한 번에 하나씩 dispatch되고 서로 다른 키는 병행하며, 빈 키(nil 메시지 또는 빈
// Room)도 하나의 직렬화 lane을 공유한다.
func StripeKey(msg *Message) string {
	return stripeKey(msg)
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
