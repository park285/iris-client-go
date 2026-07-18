package transport

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

	clientsse "github.com/park285/iris-client-go/internal/client/sse"
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

		_, _ = fmt.Fprint(w, "id: 1\ndata: {\"type\":\"member_nickname_updated\"}\n\n")
		flusher.Flush()

		_, _ = fmt.Fprint(w, "id: 2\ndata: {\"cursorStatus\":\"current\"}\n\n")
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
	if string(events[0].Data) != `{"type":"member_nickname_updated"}` {
		t.Fatalf("events[0].Data = %s, want member_nickname_updated payload", events[0].Data)
	}

	if events[1].ID != 2 {
		t.Fatalf("events[1].ID = %d, want 2", events[1].ID)
	}
	if string(events[1].Data) != `{"cursorStatus":"current"}` {
		t.Fatalf("events[1].Data = %s, want stream state payload", events[1].Data)
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

		_, _ = fmt.Fprint(w, "id: 3\ndata: {\"type\":\"member_nickname_updated\"}\n\n")
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
		_, hasHeader = r.Header[http.CanonicalHeaderKey("Last-Event-ID")]

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

func TestH2CClientEventStreamReconnectUsesLastSeenEventID(t *testing.T) {
	t.Parallel()

	var requestCount int
	var secondLastEventID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement http.Flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		switch requestCount {
		case 1:
			_, _ = fmt.Fprint(w, "id: 1\ndata: {\"type\":\"first\"}\n\n")
		case 2:
			secondLastEventID = r.Header.Get("Last-Event-ID")
			_, _ = fmt.Fprint(w, "id: 2\ndata: {\"type\":\"second\"}\n\n")
		default:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
		}
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	ch, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	var ids []int64
	for len(ids) < 2 {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("event channel closed before reconnect event")
			}
			ids = append(ids, ev.ID)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for reconnect events: %v", ctx.Err())
		}
	}

	cancel()

	if ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("event ids = %v, want [1 2]", ids)
	}
	if secondLastEventID != "1" {
		t.Fatalf("second Last-Event-ID = %q, want 1", secondLastEventID)
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

func TestSSE_TransportFailureWrapsAsTransportError(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed")
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt))
	_, err := client.EventStream(t.Context(), 0)

	assertTransportFailure(t, err)
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

		_, _ = fmt.Fprint(w, "id: 1\ndata: {\"type\":\"test\"}\n\n")
		flusher.Flush()

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

	ev, ok := <-ch
	if !ok {
		t.Fatal("channel closed before first event")
	}
	if ev.ID != 1 {
		t.Fatalf("event.ID = %d, want 1", ev.ID)
	}

	cancel()

	select {
	case <-ch:
		// 채널이 결국 닫히기만 하면 이벤트 하나를 더 받아도 허용된다.
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestParseSSEStreamEventField(t *testing.T) {
	input := "id: 1\nevent: room_event\ndata: {\"eventType\":\"member_nickname_updated\"}\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	if err := clientsse.ParseStream(ctx, scanner, ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	close(ch)

	ev := <-ch
	if ev.ID != 1 {
		t.Errorf("expected ID 1, got %d", ev.ID)
	}
	if ev.Event != SSEEventRoomEvent {
		t.Errorf("expected Event %q, got %q", SSEEventRoomEvent, ev.Event)
	}
}

func TestParseSSEStreamIgnoresComments(t *testing.T) {
	input := ": connected\n\nid: 5\ndata: {\"ok\":true}\n\n: keepalive\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	if err := clientsse.ParseStream(ctx, scanner, ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
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
	input := "id: 1\nevent: room_event\ndata: {\"a\":1}\n\nid: 2\ndata: {\"b\":2}\n\n"
	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)
	ch := make(chan RawSSEEvent, 10)

	ctx := context.Background()
	if err := clientsse.ParseStream(ctx, scanner, ch); err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	close(ch)

	ev1 := <-ch
	ev2 := <-ch

	if ev1.Event != SSEEventRoomEvent {
		t.Errorf("first event: expected %q, got %q", SSEEventRoomEvent, ev1.Event)
	}
	if ev2.Event != "" {
		t.Errorf("second event: expected empty Event (not set), got %q", ev2.Event)
	}
}

func TestParseSSEStreamScannerError(t *testing.T) {
	// 스캐너 에러 시 panic 없이 정상 종료되는지 검증
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("id: 1\ndata: {\"ok\":true}\n"))
		_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	}()

	scanner := bufio.NewScanner(pr)
	ch := make(chan RawSSEEvent, 10)
	ctx := context.Background()
	if err := clientsse.ParseStream(ctx, scanner, ch); err == nil {
		t.Fatal("parseSSEStream() error = nil, want scanner error")
	}
	close(ch)
}
