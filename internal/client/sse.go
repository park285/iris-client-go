package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

const defaultSSEScannerMaxTokenBytes = 1 << 20

type EventStreamClient interface {
	EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error)
	EventStreamReconnect(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error)
}

var _ EventStreamClient = (*H2CClient)(nil)

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
	go func() {
		defer close(out)

		nextLastEventID := drainSSEEvents(ctx, first, out, lastEventID)
		backoff := 100 * time.Millisecond
		for ctx.Err() == nil {
			if !sleepSSEReconnect(ctx, backoff) {
				return
			}
			if backoff < 2*time.Second {
				backoff *= 2
			}

			stream, err := c.eventStreamOnce(ctx, nextLastEventID)
			if err != nil {
				continue
			}

			backoff = 100 * time.Millisecond
			nextLastEventID = drainSSEEvents(ctx, stream, out, nextLastEventID)
		}
	}()

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
		return nil, &TransportError{Op: "event stream", URL: req.URL.String(), Err: err}
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		return nil, fmt.Errorf("event stream: %w", readErrorResponse(PathEventsStream, resp))
	}

	ch := make(chan RawSSEEvent, 64)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultSSEScannerMaxTokenBytes)
		if err := parseSSEStream(ctx, scanner, ch); err != nil && ctx.Err() == nil {
			c.logger.Warn("iris_sse_parse_failed", "error", err)
		}
	}()

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

func sleepSSEReconnect(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func parseSSEStream(ctx context.Context, scanner *bufio.Scanner, ch chan<- RawSSEEvent) error {
	var currentID int64
	var currentEvent string
	var data []byte
	var hasData bool

	for scanner.Scan() {
		line := scanner.Bytes()

		if len(line) == 0 {
			// 빈 줄 = 이벤트 경계
			if hasData {
				event := RawSSEEvent{
					ID:    currentID,
					Event: currentEvent,
					Data:  json.RawMessage(bytes.Clone(data)),
				}
				select {
				case ch <- event:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			currentID = 0
			currentEvent = ""
			data = data[:0]
			hasData = false
			continue
		}

		// SSE 주석 (: 로 시작) 무시
		if line[0] == ':' {
			continue
		}

		if after, ok := sseFieldValue(line, "id"); ok {
			if id, ok := parseSSEID(after); ok {
				currentID = id
			}
		} else if after, ok := sseFieldValue(line, "event"); ok {
			currentEvent = string(after)
		} else if after, ok := sseFieldValue(line, "data"); ok {
			if hasData {
				data = append(data, '\n')
			}
			data = append(data, after...)
			hasData = true
		}
	}

	return scanner.Err()
}

func sseFieldValue(line []byte, field string) ([]byte, bool) {
	n := len(field)
	if len(line) <= n || string(line[:n]) != field || line[n] != ':' {
		return nil, false
	}
	value := line[n+1:]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return value, true
}

// strconv.ParseInt(string(b), 10, 64)와 수용 범위가 동일한 []byte 파서 —
// 핫패스에서 id 라인당 string 변환 할당을 피하기 위해 직접 구현.
func parseSSEID(b []byte) (int64, bool) {
	if len(b) == 0 {
		return 0, false
	}

	neg := false
	if b[0] == '+' || b[0] == '-' {
		neg = b[0] == '-'
		b = b[1:]
		if len(b) == 0 {
			return 0, false
		}
	}

	const cutoff = math.MaxUint64/10 + 1
	var n uint64
	for _, c := range b {
		d := c - '0'
		if d > 9 {
			return 0, false
		}
		if n >= cutoff {
			return 0, false
		}
		n *= 10
		next := n + uint64(d)
		if next < n {
			return 0, false
		}
		n = next
	}

	limit := uint64(math.MaxInt64)
	if neg {
		limit++
	}
	if n > limit {
		return 0, false
	}
	if neg {
		return -int64(n), true
	}
	return int64(n), true
}
