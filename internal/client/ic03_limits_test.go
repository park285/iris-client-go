package client

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIC03RawJSONRejectsOversizeDiagnostics_201a5b77(t *testing.T) {
	t.Parallel()

	oversize := strings.Repeat("a", DefaultRawJSONMaxBytes+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+oversize+`"}`)
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "token", WithHTTPClient(srv.Client()))
	_, err := c.GetRuntimeDiagnostics(t.Context())
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("GetRuntimeDiagnostics oversize body: err = %v, want ErrResponseTooLarge", err)
	}
}

func TestIC03RawJSONAcceptsWithinLimit_201a5b77(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"state":"running"}`)
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "token", WithHTTPClient(srv.Client()))
	raw, err := c.GetRuntimeDiagnostics(t.Context())
	if err != nil {
		t.Fatalf("GetRuntimeDiagnostics within limit: err = %v", err)
	}
	if string(raw) != `{"state":"running"}` {
		t.Fatalf("body = %q, want runtime json", string(raw))
	}
}

func TestIC03SSEEventBufferCap_aaa9afe9(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	const lineSize = 256 * 1024
	chunk := strings.Repeat("x", lineSize)
	written := 0
	for written <= defaultSSEEventMaxBytes {
		b.WriteString("data: ")
		b.WriteString(chunk)
		b.WriteByte('\n')
		written += lineSize + 1
	}

	scanner := bufio.NewScanner(strings.NewReader(b.String()))
	scanner.Buffer(make([]byte, 0, 64*1024), defaultSSEScannerMaxTokenBytes)
	ch := make(chan RawSSEEvent, 4)

	err := parseSSEStream(context.Background(), scanner, ch)
	if !errors.Is(err, errSSEEventTooLarge) {
		t.Fatalf("parseSSEStream unbounded accumulation: err = %v, want errSSEEventTooLarge", err)
	}
}

func TestIC03SSESmallEventStillParses_aaa9afe9(t *testing.T) {
	t.Parallel()

	input := "id: 7\ndata: {\"ok\":true}\n\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	ch := make(chan RawSSEEvent, 2)

	if err := parseSSEStream(context.Background(), scanner, ch); err != nil {
		t.Fatalf("parseSSEStream small event: err = %v", err)
	}
	close(ch)

	ev := <-ch
	if ev.ID != 7 || string(ev.Data) != `{"ok":true}` {
		t.Fatalf("event = %+v, want id 7 with ok payload", ev)
	}
}

func TestIC03RedirectDoesNotReplaySignedPostToDifferentHost_21233857(t *testing.T) {
	t.Parallel()

	var attackerHits int
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attackerHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, attacker.URL+"/reply", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	c := NewH2CClient(origin.URL, "token", WithHTTPClient(&http.Client{
		Transport:     origin.Client().Transport,
		CheckRedirect: rejectCrossHostRedirect,
	}))

	err := c.SendMessage(t.Context(), "room", "hello")
	if err == nil {
		t.Fatal("cross-host redirect of signed POST must fail, got nil error")
	}
	if attackerHits != 0 {
		t.Fatalf("signed POST body was replayed to attacker host %d times", attackerHits)
	}
}

func TestIC03PingDrainBounded_0639c8cd(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("z", 16<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathReady {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, huge)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "token", WithHTTPClient(srv.Client()), WithPingStrategy(PingStrategyReady))
	if !c.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true (200 ready must be alive even with large body)")
	}
}
