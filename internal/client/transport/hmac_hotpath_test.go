package transport

import (
	"net/http"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

func TestSha256HexBytesEmptyIsAllocationFree(t *testing.T) {
	if got := signing.SHA256HexBytes(nil); got != signing.EmptyBodySHA256Hex {
		t.Fatalf("sha256HexBytes(nil) = %q, want %q", got, signing.EmptyBodySHA256Hex)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = signing.SHA256HexBytes(nil)
	})
	if allocs != 0 {
		t.Fatalf("sha256HexBytes(nil) allocs/run = %f, want 0", allocs)
	}
}

func BenchmarkNewSignedRequestHMACSmallJSON(b *testing.B) {
	c := NewAPIClient("http://iris.invalid", "secret", WithTransport(transportHTTP1))
	body := []byte(`{"room":"room","type":"text","data":"hello"}`)
	ctx := b.Context()

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
		_ = signing.SHA256HexBytes(nil)
	}
}
