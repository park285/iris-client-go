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

// EventStreamClient is the SSE event stream interface for Iris.
type EventStreamClient interface {
	EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error)
}

var _ EventStreamClient = (*H2CClient)(nil)

// EventStream opens an SSE connection to /events/stream and returns a channel of events.
// The channel is closed when the context is cancelled or the server closes the connection.
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
