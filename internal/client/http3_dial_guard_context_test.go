package client

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"

	"github.com/quic-go/quic-go"
)

type h3DialGuardContextKey struct{}

func TestHTTP3DialGuardContextReceivesDialContext(t *testing.T) {
	t.Parallel()

	blocked := errors.New("blocked h3 egress")
	ctx := context.WithValue(t.Context(), h3DialGuardContextKey{}, "guard-context")
	var gotValue any
	dial := guardedH3DialContext(func(ctx context.Context, ip net.IP) error {
		gotValue = ctx.Value(h3DialGuardContextKey{})
		if !ip.IsLoopback() {
			t.Fatalf("guard IP = %v, want loopback", ip)
		}
		return blocked
	})

	_, err := dial(ctx, "127.0.0.1:443", &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
	if !errors.Is(err, ErrH3EgressDenied) {
		t.Fatalf("Dial() error = %v, want ErrH3EgressDenied", err)
	}
	if !errors.Is(err, blocked) {
		t.Fatalf("Dial() error = %v, want %v", err, blocked)
	}
	if gotValue != "guard-context" {
		t.Fatalf("guard context value = %v, want guard-context", gotValue)
	}
}

func TestWithH3DialGuardContextOption(t *testing.T) {
	t.Parallel()

	guard := func(context.Context, net.IP) error { return nil }
	got := applyClientOptions([]ClientOption{WithH3DialGuardContext(guard)})
	if got.h3DialGuardContext == nil {
		t.Fatal("h3DialGuardContext was not applied")
	}
}
