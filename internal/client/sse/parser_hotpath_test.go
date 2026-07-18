package sse

import (
	"bufio"
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestParseSSEStreamMultiLineData(t *testing.T) {
	t.Parallel()

	input := "id: 9\ndata: {\"a\":1,\ndata:  \"b\":2}\n\n"
	ch := make(chan RawSSEEvent, 1)
	if err := parseSSEStream(context.Background(), bufio.NewScanner(strings.NewReader(input)), ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	close(ch)

	ev := <-ch
	if ev.ID != 9 {
		t.Fatalf("event ID = %d, want 9", ev.ID)
	}
	if string(ev.Data) != "{\"a\":1,\n \"b\":2}" {
		t.Fatalf("event data = %q, want newline-joined data lines", ev.Data)
	}
}

func TestParseSSEStreamEmptyDataLineStillEmitsEvent(t *testing.T) {
	t.Parallel()

	input := "id: 3\ndata:\n\n"
	ch := make(chan RawSSEEvent, 1)
	if err := parseSSEStream(context.Background(), bufio.NewScanner(strings.NewReader(input)), ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	close(ch)

	ev, ok := <-ch
	if !ok {
		t.Fatal("expected one event for empty data line")
	}
	if ev.ID != 3 {
		t.Fatalf("event ID = %d, want 3", ev.ID)
	}
	if len(ev.Data) != 0 {
		t.Fatalf("event data = %q, want empty", ev.Data)
	}
}

func TestParseSSEIDMatchesParseInt(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"0", "1", "42", "007", "+7", "-0", "-42",
		"9223372036854775807", "9223372036854775808",
		"-9223372036854775808", "-9223372036854775809",
		"18446744073709551616", "99999999999999999999999999",
		"", "+", "-", "abc", "1a", " 1", "1 ", "1_0", "0x10",
	} {
		want, wantErr := strconv.ParseInt(input, 10, 64)
		got, ok := parseSSEID([]byte(input))
		if ok != (wantErr == nil) {
			t.Fatalf("parseSSEID(%q) ok = %v, want %v", input, ok, wantErr == nil)
		}
		if ok && got != want {
			t.Fatalf("parseSSEID(%q) = %d, want %d", input, got, want)
		}
	}
}

const sseAllocTestEventCount = 100

func buildSSEAllocTestInput() string {
	var sb strings.Builder
	for i := range sseAllocTestEventCount {
		sb.WriteString("id: ")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("\nevent: room_event\ndata: {\"eventType\":\"member_nickname_updated\",\"seq\":")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("}\n\n")
	}
	return sb.String()
}

func TestParseSSEStreamPerEventAllocations(t *testing.T) {
	input := buildSSEAllocTestInput()
	ch := make(chan RawSSEEvent, sseAllocTestEventCount)
	ctx := context.Background()

	allocs := testing.AllocsPerRun(20, func() {
		if err := parseSSEStream(ctx, bufio.NewScanner(strings.NewReader(input)), ch); err != nil {
			t.Fatalf("parseSSEStream() error = %v", err)
		}
		for range sseAllocTestEventCount {
			<-ch
		}
	})

	// 이벤트당 예산 2 alloc(Data clone + event명) + 스캐너/리더/버퍼 셋업 여유.
	budget := float64(sseAllocTestEventCount*2 + 16)
	if allocs > budget {
		t.Fatalf("parseSSEStream allocs/run = %.0f for %d events, want <= %.0f", allocs, sseAllocTestEventCount, budget)
	}
}

func BenchmarkParseSSEStreamRoomEvents(b *testing.B) {
	input := buildSSEAllocTestInput()
	ch := make(chan RawSSEEvent, sseAllocTestEventCount)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := parseSSEStream(ctx, bufio.NewScanner(strings.NewReader(input)), ch); err != nil {
			b.Fatal(err)
		}
		for range sseAllocTestEventCount {
			<-ch
		}
	}
}
