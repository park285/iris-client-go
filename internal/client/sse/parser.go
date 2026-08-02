package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const EventMaxBytes = 8 << 20

// 이벤트 경계에서 이 상한을 넘는 누적 버퍼는 재사용하지 않고 버린다. 그러지 않으면 대형
// 이벤트 하나가 스트림이 끝날 때까지 그 크기의 배열을 붙들고 있는다.
const eventBufferRetainBytes = 64 << 10

var ErrEventTooLarge = fmt.Errorf("iris sse: accumulated event data exceeds %d bytes", EventMaxBytes)

// 라인 상한은 파서가 아니라 호출자가 준 scanner 버퍼가 소유하므로 ErrEventTooLarge와
// 분리한다. bufio.ErrTooLong 그대로 새어 나가면 호출자가 전송 실패와 구분할 수 없다.
var ErrLineTooLarge = errors.New("iris sse: single line exceeds the scanner token limit")

var internedEventNames = map[string]string{
	SSEEventRoomEvent:   SSEEventRoomEvent,
	SSEEventStreamState: SSEEventStreamState,
}

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
			data = resetEventBuffer(data)
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
			currentEvent = internEventName(after)
		} else if after, ok := sseFieldValue(line, "data"); ok {
			addition := len(after)
			if hasData {
				addition++
			}
			if len(data)+addition > EventMaxBytes {
				return ErrEventTooLarge
			}
			if hasData {
				data = append(data, '\n')
			}
			data = append(data, after...)
			hasData = true
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("%w: %w", ErrLineTooLarge, err)
		}
		return err
	}

	return nil
}

// map 조회의 string(name)은 컴파일러가 할당 없이 처리하므로, 알려진 이벤트명은 이벤트당
// 할당 하나를 없앤다.
func internEventName(name []byte) string {
	if interned, ok := internedEventNames[string(name)]; ok {
		return interned
	}
	return string(name)
}

func resetEventBuffer(data []byte) []byte {
	if cap(data) > eventBufferRetainBytes {
		return nil
	}
	return data[:0]
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
