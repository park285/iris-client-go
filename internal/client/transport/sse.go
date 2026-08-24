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
// Context가 취소되거나 서버가 연결을 닫으면 채널이 닫힙니다.
func (c *APIClient) EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	opened, err := c.eventStreamOnce(ctx, lastEventID)
	if err != nil {
		return nil, err //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
	}

	return opened.events, nil
}

// EventStreamReconnect은 /events/stream을 열고, 서버가 닫으면 마지막 수신 id로 재연결합니다.
// Context가 취소되면 반환 채널을 닫습니다.
func (c *APIClient) EventStreamReconnect(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	first, err := c.eventStreamOnce(ctx, lastEventID)
	if err != nil {
		return nil, err //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
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

func (c *APIClient) eventStreamOnce(ctx context.Context, lastEventID int64) (sseStreamOpenResult, error) {
	streamCtx, cancelStream := context.WithCancel(ctx)

	resp, err := c.connectEventStream(streamCtx, cancelStream, lastEventID)
	if err != nil {
		cancelStream()

		return sseStreamOpenResult{}, err //nolint:wrapcheck // connectEventStream이 event stream 맥락으로 이미 래핑한다.
	}

	if resp.StatusCode == http.StatusNoContent {
		defer cancelStream()
		defer resp.Body.Close()

		return sseStreamOpenResult{events: closedSSEEvents(), terminal: true}, nil
	}

	ch := make(chan RawSSEEvent, 64)

	safeGo(c.logger, "iris_sse_reader_panic_recovered", func() {
		c.readEventStream(ctx, resp, ch, cancelStream)
	})

	return sseStreamOpenResult{events: ch}, nil
}

// connectEventStream은 connect timeout 안에서 응답 상태 확인까지 마치고, 204 또는 성공 응답만 돌려준다.
func (c *APIClient) connectEventStream(streamCtx context.Context, cancelStream context.CancelFunc, lastEventID int64) (*http.Response, error) {
	req, err := c.newSignedRequest(streamCtx, http.MethodGet, PathEventsStream, nil, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("event stream: %w", err)
	}

	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(lastEventID, 10))
	}

	stopConnectTimer := c.startConnectTimer(cancelStream)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		stopConnectTimer()

		return nil, &TransportError{Op: "event stream", URL: redactedURLForError(req.URL.String()), Err: err}
	}

	if resp.StatusCode == http.StatusNoContent {
		stopConnectTimer()

		return resp, nil
	}

	// 오류 본문 읽기도 connect deadline 안에서 끝나야 하므로 여기서는 타이머를 미리 멈추지 않는다.
	if !isSuccessfulHTTPStatus(resp.StatusCode) {
		defer stopConnectTimer()
		defer resp.Body.Close()

		return nil, fmt.Errorf("event stream: %w", readErrorResponse(PathEventsStream, resp))
	}

	// Stop이 false면 타이머가 이미 fire해 streamCtx를 취소했다는 뜻이므로, 곧 끊길 body를
	// 성립된 스트림으로 넘기지 않는다.
	if !stopConnectTimer() {
		defer resp.Body.Close()

		return nil, &TransportError{
			Op:  "event stream",
			URL: redactedURLForError(req.URL.String()),
			Err: fmt.Errorf("connect timeout elapsed before the stream was handed off: %w", context.DeadlineExceeded),
		}
	}

	return resp, nil
}

func (c *APIClient) startConnectTimer(cancelStream context.CancelFunc) func() bool {
	if c.client.Timeout <= 0 {
		return func() bool { return true }
	}

	return time.AfterFunc(c.client.Timeout, cancelStream).Stop
}

func (c *APIClient) readEventStream(ctx context.Context, resp *http.Response, ch chan RawSSEEvent, cancelStream context.CancelFunc) {
	defer close(ch)
	defer cancelStream()
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), defaultSSEScannerMaxTokenBytes)

	if err := clientsse.ParseStream(ctx, scanner, ch); err != nil && ctx.Err() == nil {
		c.logger.Warn("iris_sse_parse_failed", "error", err)
	}
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
