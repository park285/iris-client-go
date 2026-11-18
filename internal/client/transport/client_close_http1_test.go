package transport

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseReleasesOwnedHTTP1IdleConnection(t *testing.T) {
	t.Parallel()

	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	client, err := NewClient(WithBaseURL(server.URL), WithBotToken("token"), WithTransport(TransportHTTP1))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})

	if err := client.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close left the owned HTTP/1 connection idle")
	}
}

func TestClosePreservesCallerHTTP1IdleConnection(t *testing.T) {
	t.Parallel()

	var connections atomic.Int32

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	httpClient := &http.Client{Transport: newHTTP1Transport()}
	t.Cleanup(httpClient.CloseIdleConnections)

	client, err := NewClient(WithBaseURL(server.URL), WithBotToken("token"), WithHTTPClient(httpClient))
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	if err := client.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := connections.Load(); got != 1 {
		t.Fatalf("connection count = %d, want one reused caller connection", got)
	}
}
