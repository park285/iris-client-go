package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
)

const EventMaxBytes = 8 << 20
const defaultSSEEventMaxBytes = EventMaxBytes

var ErrEventTooLarge = fmt.Errorf("iris sse: accumulated event data exceeds %d bytes", defaultSSEEventMaxBytes)
var errSSEEventTooLarge = ErrEventTooLarge

func ParseStream(ctx context.Context, scanner *bufio.Scanner, ch chan<- RawSSEEvent) error {
	return parseSSEStream(ctx, scanner, ch)
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
			addition := len(after)
			if hasData {
				addition++
			}
			if len(data)+addition > defaultSSEEventMaxBytes {
				return errSSEEventTooLarge
			}
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

// parseSSEID는 strconv.ParseInt와 같은 범위를 받되 string 할당을 피한다.
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
