package client

import (
	"bufio"
	"context"
	"errors"
	"io"
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
