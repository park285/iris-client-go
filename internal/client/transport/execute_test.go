package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoSignedInitErrorIsNotRetryable(t *testing.T) {
	c := NewH2CClient("https://iris.invalid", "token",
		WithTransport("h3"), WithH3CACertFile("/nonexistent/ca.pem"))
	if c.InitError() == nil {
		t.Fatal("expected init error")
	}

	_, err := c.doSigned(context.Background(), http.MethodGet, PathConfig, SecretRoleInbound)
	var te *TransportError
	if !errors.As(err, &te) || te.Op != opInit {
		t.Fatalf("want TransportError{Op:init}, got %v", err)
	}
	if errors.Is(err, ErrRetryable) {
		t.Fatal("init error must not be retryable")
	}
}

func TestDoSignedRejectsNon2xxStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "token", WithHTTPClient(server.Client()))
	resp, err := c.doSigned(t.Context(), http.MethodGet, PathDiagnosticsRuntime, SecretRoleBotControl)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("doSigned() status 304 error = nil, want non-2xx failure")
	}
}
