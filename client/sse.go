package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type EventStreamClient interface {
	EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error)
}

var _ EventStreamClient = (*H2CClient)(nil)

// EventStream은 /events/stream에 SSE 연결을 열고 이벤트 채널을 반환합니다.
// context가 취소되거나 서버가 연결을 닫으면 채널이 닫힙니다.
func (c *H2CClient) EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error) {
	req, err := c.newSignedRequest(ctx, http.MethodGet, PathEventsStream, nil)
	if err != nil {
		return nil, fmt.Errorf("event stream: %w", err)
	}

	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(lastEventID, 10))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("event stream: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("event stream: %w", readErrorResponse(PathEventsStream, resp))
	}

	ch := make(chan RawSSEEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parseSSEStream(ctx, bufio.NewScanner(resp.Body), ch)
	}()

	return ch, nil
}

func parseSSEStream(ctx context.Context, scanner *bufio.Scanner, ch chan<- RawSSEEvent) {
	var currentID int64
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line = event boundary
			if len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				event := RawSSEEvent{
					ID:   currentID,
					Data: json.RawMessage(data),
				}
				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}
			}
			currentID = 0
			dataLines = dataLines[:0]
			continue
		}

		if strings.HasPrefix(line, "id: ") {
			if id, err := strconv.ParseInt(strings.TrimPrefix(line, "id: "), 10, 64); err == nil {
				currentID = id
			}
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
}
