package sse

import (
	"bufio"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestParseSSEStreamAcceptsFieldsWithoutSpace(t *testing.T) {
	t.Parallel()

	input := "id:7\nevent:room_event\ndata:{\"ok\":true}\n\n"
	ch := make(chan RawSSEEvent, 1)
	err := parseSSEStream(context.Background(), bufio.NewScanner(strings.NewReader(input)), ch)
	close(ch)
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}

	ev := <-ch
	if ev.ID != 7 {
		t.Fatalf("event ID = %d, want 7", ev.ID)
	}
	if ev.Event != SSEEventRoomEvent {
		t.Fatalf("event name = %q, want %s", ev.Event, SSEEventRoomEvent)
	}
	if string(ev.Data) != `{"ok":true}` {
		t.Fatalf("event data = %s, want compact JSON", ev.Data)
	}
}

func TestParseSSEStreamReturnsScannerError(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("id: 1\ndata: {\"ok\":true}\n"))
		_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	}()

	err := parseSSEStream(context.Background(), bufio.NewScanner(pr), make(chan RawSSEEvent, 1))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("parseSSEStream() error = %v, want ErrUnexpectedEOF", err)
	}
}

func TestSSEFieldValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		line string
		want string
	}{
		{line: "data: value", want: "value"},
		{line: "data:value", want: "value"},
		{line: "data:  value", want: " value"},
	} {
		got, ok := sseFieldValue([]byte(tc.line), "data")
		if !ok || string(got) != tc.want {
			t.Fatalf("sseFieldValue(%q) = %q,%v want %q,true", tc.line, got, ok, tc.want)
		}
	}
}

func TestParseSSEStreamMapsScannerTokenOverflow(t *testing.T) {
	t.Parallel()

	scanner := bufio.NewScanner(strings.NewReader("data: " + strings.Repeat("x", 4096) + "\n\n"))
	scanner.Buffer(make([]byte, 0, 64), 128)

	err := parseSSEStream(context.Background(), scanner, make(chan RawSSEEvent, 1))
	if !errors.Is(err, ErrLineTooLarge) {
		t.Fatalf("parseSSEStream() error = %v, want ErrLineTooLarge", err)
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("parseSSEStream() error = %v, want the bufio cause preserved", err)
	}
}

func TestParseSSEStreamInternsKnownEventNames(t *testing.T) {
	t.Parallel()

	input := "event: " + SSEEventRoomEvent + "\ndata: {}\n\n" +
		"event: " + SSEEventStreamState + "\ndata: {}\n\n" +
		"event: unknown_event\ndata: {}\n\n"
	ch := make(chan RawSSEEvent, 3)
	if err := parseSSEStream(context.Background(), bufio.NewScanner(strings.NewReader(input)), ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	close(ch)

	var names []string
	for ev := range ch {
		names = append(names, ev.Event)
	}
	want := []string{SSEEventRoomEvent, SSEEventStreamState, "unknown_event"}
	if !slices.Equal(names, want) {
		t.Fatalf("event names = %v, want %v", names, want)
	}
}

func TestInternEventNameIsAllocationFreeForKnownNames(t *testing.T) {
	allocs := testing.AllocsPerRun(200, func() {
		if got := internEventName([]byte(SSEEventRoomEvent)); got != SSEEventRoomEvent {
			t.Fatalf("internEventName = %q", got)
		}
	})
	if allocs != 0 {
		t.Fatalf("internEventName allocs/run = %.0f for a known name, want 0", allocs)
	}
}

func TestResetEventBufferReleasesOversizedBacking(t *testing.T) {
	t.Parallel()

	small := make([]byte, 0, eventBufferRetainBytes)
	if got := resetEventBuffer(append(small, 'a')); cap(got) != eventBufferRetainBytes {
		t.Fatalf("cap after reset = %d, want the buffer at the retain threshold reused (%d)", cap(got), eventBufferRetainBytes)
	}

	large := make([]byte, 0, eventBufferRetainBytes+1)
	if got := resetEventBuffer(append(large, 'a')); cap(got) != 0 {
		t.Fatalf("cap after reset = %d, want an oversized buffer released", cap(got))
	}
}
