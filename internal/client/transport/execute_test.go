package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoSignedInitErrorIsNotRetryable(t *testing.T) {
	c := NewAPIClient("https://iris.invalid", "token",
		WithTransport("h3"), WithH3CACertFile("/nonexistent/ca.pem"))
	if c.InitError() == nil {
		t.Fatal("expected init error")
	}

	resp, err := c.doSigned(t.Context(), http.MethodGet, PathConfig, SecretRoleInbound)
	if resp != nil {
		defer resp.Body.Close()

		t.Fatalf("doSigned() response = %v, want nil on init error", resp)
	}

	te, ok := errors.AsType[*TransportError](err)
	if !ok || te.Op != opInit {
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

	c := NewAPIClient(server.URL, "token", WithHTTPClient(server.Client()))

	resp, err := c.doSigned(t.Context(), http.MethodGet, PathDiagnosticsRuntime, SecretRoleBotControl)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}

		t.Fatal("doSigned() status 304 error = nil, want non-2xx failure")
	}
}
