package transport

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	clientsse "github.com/park285/iris-client-go/internal/client/sse"
)

const (
	defaultSSEScannerMaxTokenBytes = 1 << 20
	sseReconnectInitialBackoff     = 100 * time.Millisecond
	sseReconnectMaxBackoff         = 2 * time.Second
)

// EventStream은 /events/stream에 SSE 연결을 열고 이벤트 채널을 반환합니다.
// context가 취소되거나 서버가 연결을 닫으면 채널이 닫힙니다.
func (c *H2CClient) EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	return c.eventStreamOnce(ctx, lastEventID)
}

// EventStreamReconnect은 /events/stream을 열고, 서버가 닫으면 마지막 수신 id로 재연결합니다.
// context가 취소되면 반환 채널을 닫습니다.
func (c *H2CClient) EventStreamReconnect(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	first, err := c.eventStreamOnce(ctx, lastEventID)
	if err != nil {
		return nil, err
	}

	out := make(chan RawSSEEvent, 64)
	safeGo(c.logger, "iris_sse_reconnect_panic_recovered", func() {
		defer close(out)

		nextLastEventID := drainSSEEvents(ctx, first, out, lastEventID)
		backoff := sseReconnectInitialBackoff
		attempt := 0
		lastError := ""
		for ctx.Err() == nil {
			if !waitRetryDelay(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, sseReconnectMaxBackoff)
			attempt++
			c.opts.TransportMetrics.ObserveSSEReconnectAttempt(attempt)

			stream, err := c.eventStreamOnce(ctx, nextLastEventID)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.opts.TransportMetrics.ObserveSSEReconnectFailure(attempt)
				if err.Error() != lastError {
					c.logger.Warn("iris_sse_reconnect_failed", "attempt", attempt, "error", err)
					lastError = err.Error()
				}
				continue
			}

			c.opts.TransportMetrics.ObserveSSEReconnectSuccess(attempt)
			backoff = sseReconnectInitialBackoff
			attempt = 0
			lastError = ""
			nextLastEventID = drainSSEEvents(ctx, stream, out, nextLastEventID)
		}
	})

	return out, nil
}

func (c *H2CClient) eventStreamOnce(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	req, err := c.newSignedRequest(ctx, http.MethodGet, PathEventsStream, nil, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("event stream: %w", err)
	}

	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(lastEventID, 10))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: "event stream", URL: redactedURLForError(req.URL.String()), Err: err}
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		return nil, fmt.Errorf("event stream: %w", readErrorResponse(PathEventsStream, resp))
	}

	ch := make(chan RawSSEEvent, 64)
	safeGo(c.logger, "iris_sse_reader_panic_recovered", func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultSSEScannerMaxTokenBytes)
		if err := clientsse.ParseStream(ctx, scanner, ch); err != nil && ctx.Err() == nil {
			c.logger.Warn("iris_sse_parse_failed", "error", err)
		}
	})

	return ch, nil
}

func drainSSEEvents(ctx context.Context, stream <-chan RawSSEEvent, out chan<- RawSSEEvent, lastEventID int64) int64 {
	nextLastEventID := lastEventID
	for ev := range stream {
		if ev.ID > 0 {
			nextLastEventID = ev.ID
		}
		select {
		case out <- ev:
		case <-ctx.Done():
			return nextLastEventID
		}
	}

	return nextLastEventID
}
