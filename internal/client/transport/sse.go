package transport

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	clientsse "github.com/park285/iris-client-go/v2/internal/client/sse"
)

const (
	defaultSSEScannerMaxTokenBytes = 1 << 20
	sseReconnectInitialBackoff     = 100 * time.Millisecond
	sseReconnectMaxBackoff         = 2 * time.Second
)

type sseStreamOpenResult struct {
	events   <-chan RawSSEEvent
	terminal bool
}

type sseStreamDrainResult struct {
	lastEventID int64
	eventCount  int
}

// EventStream은 /events/stream에 SSE 연결을 열고 이벤트 채널을 반환합니다.
// context가 취소되거나 서버가 연결을 닫으면 채널이 닫힙니다.
func (c *H2CClient) EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	opened, err := c.eventStreamOnce(ctx, lastEventID)
	if err != nil {
		return nil, err
	}
	return opened.events, nil
}

// EventStreamReconnect은 /events/stream을 열고, 서버가 닫으면 마지막 수신 id로 재연결합니다.
// context가 취소되면 반환 채널을 닫습니다.
func (c *H2CClient) EventStreamReconnect(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	first, err := c.eventStreamOnce(ctx, lastEventID)
	if err != nil {
		return nil, err
	}
	if first.terminal {
		return first.events, nil
	}

	out := make(chan RawSSEEvent, 64)
	safeGo(c.logger, "iris_sse_reconnect_panic_recovered", func() {
		defer close(out)

		drained := drainSSEEvents(ctx, first.events, out, lastEventID)
		nextLastEventID := drained.lastEventID
		backoff := sseReconnectInitialBackoff
		attempt := 0
		lastError := ""
		for ctx.Err() == nil {
			if !waitRetryDelay(ctx, backoff) {
				return
			}
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
				backoff = nextBackoff(backoff, sseReconnectMaxBackoff)
				continue
			}

			c.opts.TransportMetrics.ObserveSSEReconnectSuccess(attempt)
			if stream.terminal {
				return
			}
			attempt = 0
			lastError = ""
			drained = drainSSEEvents(ctx, stream.events, out, nextLastEventID)
			nextLastEventID = drained.lastEventID
			backoff = sseReconnectBackoffAfterDrain(backoff, drained.eventCount)
		}
	})

	return out, nil
}

func sseReconnectBackoffAfterDrain(current time.Duration, eventCount int) time.Duration {
	if eventCount == 0 {
		return nextBackoff(current, sseReconnectMaxBackoff)
	}
	return sseReconnectInitialBackoff
}

func (c *H2CClient) eventStreamOnce(ctx context.Context, lastEventID int64) (sseStreamOpenResult, error) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	req, err := c.newSignedRequest(streamCtx, http.MethodGet, PathEventsStream, nil, SecretRoleBotControl)
	if err != nil {
		cancelStream()
		return sseStreamOpenResult{}, fmt.Errorf("event stream: %w", err)
	}

	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(lastEventID, 10))
	}

	var connectTimer *time.Timer
	if c.client.Timeout > 0 {
		connectTimer = time.AfterFunc(c.client.Timeout, cancelStream)
	}
	stopConnectTimer := func() bool {
		if connectTimer == nil {
			return true
		}
		return connectTimer.Stop()
	}

	resp, err := c.streamClient.Do(req)
	if err != nil {
		stopConnectTimer()
		cancelStream()
		return sseStreamOpenResult{}, &TransportError{Op: "event stream", URL: redactedURLForError(req.URL.String()), Err: err}
	}

	if resp.StatusCode == http.StatusNoContent {
		stopConnectTimer()
		_ = resp.Body.Close()
		cancelStream()
		return sseStreamOpenResult{events: closedSSEEvents(), terminal: true}, nil
	}
	// 오류 본문 읽기도 connect deadline 안에서 끝나야 하므로 여기서는 타이머를 미리 멈추지 않는다.
	if !isSuccessfulHTTPStatus(resp.StatusCode) {
		defer stopConnectTimer()
		defer cancelStream()
		defer func() { _ = resp.Body.Close() }()
		return sseStreamOpenResult{}, fmt.Errorf("event stream: %w", readErrorResponse(PathEventsStream, resp))
	}

	// Stop이 false면 타이머가 이미 fire해 streamCtx를 취소했다는 뜻이므로, 곧 끊길 body를
	// 성립된 스트림으로 넘기지 않는다.
	if !stopConnectTimer() {
		_ = resp.Body.Close()
		cancelStream()
		return sseStreamOpenResult{}, &TransportError{
			Op:  "event stream",
			URL: redactedURLForError(req.URL.String()),
			Err: fmt.Errorf("connect timeout elapsed before the stream was handed off: %w", context.DeadlineExceeded),
		}
	}

	ch := make(chan RawSSEEvent, 64)
	safeGo(c.logger, "iris_sse_reader_panic_recovered", func() {
		defer close(ch)
		defer cancelStream()
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultSSEScannerMaxTokenBytes)
		if err := clientsse.ParseStream(ctx, scanner, ch); err != nil && ctx.Err() == nil {
			c.logger.Warn("iris_sse_parse_failed", "error", err)
		}
	})

	return sseStreamOpenResult{events: ch}, nil
}

func closedSSEEvents() <-chan RawSSEEvent {
	ch := make(chan RawSSEEvent)
	close(ch)
	return ch
}

func drainSSEEvents(ctx context.Context, stream <-chan RawSSEEvent, out chan<- RawSSEEvent, lastEventID int64) sseStreamDrainResult {
	result := sseStreamDrainResult{lastEventID: lastEventID}
	for ev := range stream {
		if ev.ID > 0 {
			result.lastEventID = ev.ID
		}
		select {
		case out <- ev:
			result.eventCount++
		case <-ctx.Done():
			return result
		}
	}

	return result
}
