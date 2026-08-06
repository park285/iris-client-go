package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

func TestH2CClientEventStreamReconnectLogsRepeatedFailureOnce(t *testing.T) {
	var logs bytes.Buffer
	var requestCount atomic.Int32
	reconnectBlocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requestCount.Add(1) {
		case 1:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case 2, 3:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case 4:
			close(reconnectBlocked)
			<-r.Context().Done()
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(
		server.URL,
		"",
		WithTransport("http1"),
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
	)
	ch, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	waitForSSEReconnect(t, reconnectBlocked)
	cancel()
	waitForSSEChannelClose(t, ch)

	records := decodeSSEReconnectFailureLogs(t, logs.String())
	if len(records) != 1 {
		t.Fatalf("reconnect failure log count = %d, want 1; logs = %s", len(records), logs.String())
	}
	if records[0].Attempt != 1 {
		t.Fatalf("reconnect failure attempt = %d, want 1", records[0].Attempt)
	}
	if !strings.Contains(records[0].Error, "401") {
		t.Fatalf("reconnect failure error = %q, want 401 mention", records[0].Error)
	}
}

func TestH2CClientEventStreamReconnectResetsFailureSuppressionAfterSuccess(t *testing.T) {
	var logs bytes.Buffer
	var requestCount atomic.Int32
	reconnectBlocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requestCount.Add(1) {
		case 1, 3:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case 2, 4:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case 5:
			close(reconnectBlocked)
			<-r.Context().Done()
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(
		server.URL,
		"",
		WithTransport("http1"),
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
	)
	ch, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	waitForSSEReconnect(t, reconnectBlocked)
	cancel()
	waitForSSEChannelClose(t, ch)

	records := decodeSSEReconnectFailureLogs(t, logs.String())
	if len(records) != 2 {
		t.Fatalf("reconnect failure log count = %d, want 2; logs = %s", len(records), logs.String())
	}
	for i, record := range records {
		if record.Attempt != 1 {
			t.Fatalf("reconnect failure log %d attempt = %d, want 1", i, record.Attempt)
		}
		if !strings.Contains(record.Error, "401") {
			t.Fatalf("reconnect failure log %d error = %q, want 401 mention", i, record.Error)
		}
	}
}

func TestH2CClientEventStreamReconnectDoesNotLogContextCancellation(t *testing.T) {
	var logs bytes.Buffer
	var requestCount atomic.Int32
	reconnectBlocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			return
		}
		close(reconnectBlocked)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	client := NewH2CClient(
		server.URL,
		"",
		WithTransport("http1"),
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
	)
	ch, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	waitForSSEReconnect(t, reconnectBlocked)
	cancel()
	waitForSSEChannelClose(t, ch)

	if records := decodeSSEReconnectFailureLogs(t, logs.String()); len(records) != 0 {
		t.Fatalf("reconnect failure log count = %d, want 0; logs = %s", len(records), logs.String())
	}
}

type sseReconnectFailureLog struct {
	Message string `json:"msg"`
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
}

func decodeSSEReconnectFailureLogs(t *testing.T, output string) []sseReconnectFailureLog {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	var records []sseReconnectFailureLog
	for {
		var record sseReconnectFailureLog
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				return records
			}
			t.Fatalf("decode reconnect log: %v", err)
		}
		if record.Message == "iris_sse_reconnect_failed" {
			records = append(records, record)
		}
	}
}

func waitForSSEReconnect(t *testing.T, reconnectBlocked <-chan struct{}) {
	t.Helper()

	select {
	case <-reconnectBlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE reconnect")
	}
}

func waitForSSEChannelClose(t *testing.T, ch <-chan RawSSEEvent) {
	t.Helper()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("event channel emitted an unexpected event")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event channel close")
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

func TestH2CClientEventStreamBodyOutlivesClientTimeout(t *testing.T) {
	t.Parallel()

	const requestTimeout = 500 * time.Millisecond
	releaseEvent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement http.Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		<-releaseEvent
		_, _ = fmt.Fprint(w, "id: 9\ndata: {\"type\":\"late\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseEvent) })
	}
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := NewH2CClient(server.URL, "token", WithTransport("http1"), WithTimeout(requestTimeout))
	stream, err := client.EventStream(ctx, 0)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}

	timer := time.NewTimer(3 * requestTimeout)
	defer timer.Stop()
	<-timer.C
	release()

	select {
	case event, ok := <-stream:
		if !ok {
			t.Fatal("stream closed at unary client timeout")
		}
		if event.ID != 9 {
			t.Fatalf("event ID = %d, want 9", event.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event after unary client timeout")
	}
}

func TestH2CClientEventStreamHeadersHonorClientTimeout(t *testing.T) {
	t.Parallel()

	const requestTimeout = 40 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "token", WithTransport("http1"), WithTimeout(requestTimeout))
	started := time.Now()
	_, err := client.EventStream(t.Context(), 0)
	if err == nil {
		t.Fatal("EventStream() error = nil, want response-header timeout")
	}
	if elapsed := time.Since(started); elapsed > 10*requestTimeout {
		t.Fatalf("EventStream() elapsed = %s, want bounded header wait", elapsed)
	}
}

func TestH2CClientEventStreamHeadersHonorInjectedClientTimeout(t *testing.T) {
	t.Parallel()

	const requestTimeout = 40 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	injected := server.Client()
	injected.Timeout = requestTimeout
	client := NewH2CClient(server.URL, "token", WithHTTPClient(injected))
	started := time.Now()
	_, err := client.EventStream(t.Context(), 0)
	if err == nil {
		t.Fatal("EventStream() error = nil, want injected-client header timeout")
	}
	if elapsed := time.Since(started); elapsed > 10*requestTimeout {
		t.Fatalf("EventStream() elapsed = %s, want injected-client timeout bound", elapsed)
	}
	if injected.Timeout != requestTimeout {
		t.Fatalf("injected client timeout = %s, want unchanged %s", injected.Timeout, requestTimeout)
	}
}

func TestH2CClientEventStreamErrorBodyHonorsClientTimeout(t *testing.T) {
	t.Parallel()

	const requestTimeout = 40 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "token", WithTransport("http1"), WithTimeout(requestTimeout))
	started := time.Now()
	_, err := client.EventStream(t.Context(), 0)
	if err == nil {
		t.Fatal("EventStream() error = nil, want non-2xx error")
	}
	if elapsed := time.Since(started); elapsed > 10*requestTimeout {
		t.Fatalf("EventStream() elapsed = %s, want bounded error-body read", elapsed)
	}
}

func TestH2CClientEventStreamRejectsNon2xxStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultipleChoices)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "token", WithTransport("http1"))
	if _, err := client.EventStream(t.Context(), 0); err == nil {
		t.Fatal("EventStream() status 300 error = nil, want non-2xx failure")
	}
}

func TestH2CClientEventStreamReconnectStopsOnNoContent(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "id: 1\ndata: {\"type\":\"first\"}\n\n")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	client := NewH2CClient(server.URL, "token", WithTransport("http1"))
	stream, err := client.EventStreamReconnect(ctx, 0)
	if err != nil {
		t.Fatalf("EventStreamReconnect() error = %v", err)
	}

	event, ok := <-stream
	if !ok || event.ID != 1 {
		t.Fatalf("first event = %+v, ok = %v, want ID 1", event, ok)
	}
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("stream emitted an event after terminal 204")
		}
	case <-ctx.Done():
		t.Fatalf("stream did not close after terminal 204: %v", ctx.Err())
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
}

func TestH2CClientEventStreamNoContentReturnsClosedChannel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "token", WithTransport("http1"))
	stream, err := client.EventStream(t.Context(), 0)
	if err != nil {
		t.Fatalf("EventStream() error = %v", err)
	}
	if _, ok := <-stream; ok {
		t.Fatal("EventStream() 204 channel is open, want normally closed channel")
	}
}

func TestSSEReconnectBackoffPacesEmptyStreams(t *testing.T) {
	t.Parallel()

	backoff := sseReconnectInitialBackoff
	for _, want := range []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond} {
		backoff = sseReconnectBackoffAfterDrain(backoff, 0)
		if backoff != want {
			t.Fatalf("empty stream backoff = %s, want %s", backoff, want)
		}
	}
	if got := sseReconnectBackoffAfterDrain(backoff, 1); got != sseReconnectInitialBackoff {
		t.Fatalf("event-bearing stream backoff = %s, want %s", got, sseReconnectInitialBackoff)
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

// connect 타이머가 Do 반환과 stop 사이에 fire하면 streamCtx는 이미 취소되어 있다.
// RoundTripper가 그 취소를 기다렸다 성공을 돌려주므로 그 창을 결정론적으로 재현한다.
func TestEventStreamRejectsStreamCancelledByConnectTimer(t *testing.T) {
	t.Parallel()

	var bodyClosed atomic.Bool
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeTrackingBody{Reader: strings.NewReader("data: {}\n\n"), closed: &bodyClosed},
		}, nil
	})

	c := NewH2CClient("https://iris.test", "token", WithRoundTripper(rt), WithTimeout(20*time.Millisecond))

	events, err := c.EventStream(t.Context(), 0)
	if err == nil {
		t.Fatal("EventStream() error = nil; a stream whose context the connect timer already cancelled must not be handed off")
	}
	if events != nil {
		t.Fatal("EventStream() returned a channel alongside the error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EventStream() error = %v, want context.DeadlineExceeded", err)
	}
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("EventStream() error = %v, want ErrTransport classification", err)
	}
	if !bodyClosed.Load() {
		t.Fatal("EventStream() leaked the response body of the abandoned stream")
	}
}

type closeTrackingBody struct {
	*strings.Reader
	closed *atomic.Bool
}

func (b closeTrackingBody) Close() error {
	b.closed.Store(true)
	return nil
}
