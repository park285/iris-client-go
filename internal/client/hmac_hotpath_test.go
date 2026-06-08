package client

import (
	"context"
	"net/http"
	"testing"
)

func TestSha256HexBytesEmptyIsAllocationFree(t *testing.T) {
	if got := sha256HexBytes(nil); got != emptyBodySHA256Hex {
		t.Fatalf("sha256HexBytes(nil) = %q, want %q", got, emptyBodySHA256Hex)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = sha256HexBytes(nil)
	})
	if allocs != 0 {
		t.Fatalf("sha256HexBytes(nil) allocs/run = %f, want 0", allocs)
	}
}

func BenchmarkNewSignedRequestHMACSmallJSON(b *testing.B) {
	c := NewH2CClient("http://iris.invalid", "secret", WithTransport("http1"))
	body := []byte(`{"room":"room","type":"text","data":"hello"}`)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.newSignedRequest(ctx, http.MethodPost, PathReply, body, SecretRoleBotControl); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSha256HexBytesEmpty(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = sha256HexBytes(nil)
	}
}
