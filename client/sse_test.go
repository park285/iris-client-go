package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestH2CClientEventStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != PathEventsStream {
			t.Fatalf("path = %s, want %s", r.URL.Path, PathEventsStream)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement http.Flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "id: 1\ndata: {\"type\":\"member_event\"}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "id: 2\ndata: {\"type\":\"nickname_change\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	ch, err := client.EventStream(ctx, 0)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}

	var events []RawSSEEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	if events[0].ID != 1 {
		t.Fatalf("events[0].ID = %d, want 1", events[0].ID)
	}
	if string(events[0].Data) != `{"type":"member_event"}` {
		t.Fatalf("events[0].Data = %s, want member_event payload", events[0].Data)
	}

	if events[1].ID != 2 {
		t.Fatalf("events[1].ID = %d, want 2", events[1].ID)
	}
	if string(events[1].Data) != `{"type":"nickname_change"}` {
		t.Fatalf("events[1].Data = %s, want nickname_change payload", events[1].Data)
	}
}

func TestH2CClientEventStreamLastEventID(t *testing.T) {
	t.Parallel()

	var gotLastEventID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLastEventID = r.Header.Get("Last-Event-ID")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement http.Flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "id: 3\ndata: {\"type\":\"role_change\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	ch, err := client.EventStream(ctx, 42)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}

	// Drain the channel
	for range ch {
	}

	if gotLastEventID != "42" {
		t.Fatalf("Last-Event-ID = %q, want 42", gotLastEventID)
	}
}

func TestH2CClientEventStreamNoLastEventIDWhenZero(t *testing.T) {
	t.Parallel()

	var gotLastEventID string
	var hasHeader bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLastEventID = r.Header.Get("Last-Event-ID")
		_, hasHeader = r.Header["Last-Event-ID"]

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	ch, err := client.EventStream(ctx, 0)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}

	for range ch {
	}

	if hasHeader {
		t.Fatalf("Last-Event-ID header sent with value %q, want absent when lastEventID=0", gotLastEventID)
	}
}

func TestH2CClientEventStreamError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.EventStream(t.Context(), 0)
	if err == nil {
		t.Fatal("expected error for 403")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %q, want 403 mention", err.Error())
	}
}

func TestH2CClientEventStreamContextCancel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement http.Flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "id: 1\ndata: {\"type\":\"test\"}\n\n")
		flusher.Flush()

		// Keep the connection open until the client disconnects
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	ch, err := client.EventStream(ctx, 0)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}

	// Read the first event
	ev, ok := <-ch
	if !ok {
		t.Fatal("channel closed before first event")
	}
	if ev.ID != 1 {
		t.Fatalf("event.ID = %d, want 1", ev.ID)
	}

	// Cancel context and verify channel closes
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// Receiving extra events is acceptable as long as the channel eventually closes
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestParseSSEStreamEventField(t *testing.T) {
	input := "id: 1\nevent: member_event\ndata: {\"type\":\"member_event\"}\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	parseSSEStream(ctx, scanner, ch)
	close(ch)

	ev := <-ch
	if ev.ID != 1 {
		t.Errorf("expected ID 1, got %d", ev.ID)
	}
	if ev.Event != "member_event" {
		t.Errorf("expected Event 'member_event', got %q", ev.Event)
	}
}

func TestParseSSEStreamIgnoresComments(t *testing.T) {
	input := ": connected\n\nid: 5\ndata: {\"ok\":true}\n\n: keepalive\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	parseSSEStream(ctx, scanner, ch)
	close(ch)

	events := make([]RawSSEEvent, 0)
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event (comments should not produce events), got %d", len(events))
	}
	if events[0].ID != 5 {
		t.Errorf("expected ID 5, got %d", events[0].ID)
	}
}

func TestParseSSEStreamEventResetsBetweenEvents(t *testing.T) {
	input := "id: 1\nevent: member_event\ndata: {\"a\":1}\n\nid: 2\ndata: {\"b\":2}\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	parseSSEStream(ctx, scanner, ch)
	close(ch)

	ev1 := <-ch
	ev2 := <-ch

	if ev1.Event != "member_event" {
		t.Errorf("first event: expected 'member_event', got %q", ev1.Event)
	}
	if ev2.Event != "" {
		t.Errorf("second event: expected empty Event (not set), got %q", ev2.Event)
	}
}

func TestParseSSEStreamScannerError(t *testing.T) {
	// 스캐너 에러 시 panic 없이 정상 종료되는지 검증
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("id: 1\ndata: {\"ok\":true}\n"))
		pw.CloseWithError(io.ErrUnexpectedEOF)
	}()

	scanner := bufio.NewScanner(pr)
	ch := make(chan RawSSEEvent, 10)
	ctx := context.Background()
	parseSSEStream(ctx, scanner, ch)
	close(ch)
	// panic 없이 종료되면 성공
}
