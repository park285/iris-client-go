package client

import (
	"context"
	"errors"
	"net/http"
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
