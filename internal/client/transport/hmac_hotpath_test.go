package transport

import (
	"net/http"
	"testing"

	"github.com/park285/iris-client-go/v6/internal/irishmac"
)

func TestSha256HexBytesEmptyIsAllocationFree(t *testing.T) {
	if got := irishmac.SHA256HexBytes(nil); got != irishmac.EmptyBodySHA256Hex {
		t.Fatalf("sha256HexBytes(nil) = %q, want %q", got, irishmac.EmptyBodySHA256Hex)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = irishmac.SHA256HexBytes(nil)
	})
	if allocs != 0 {
		t.Fatalf("sha256HexBytes(nil) allocs/run = %f, want 0", allocs)
	}
}

func BenchmarkNewSignedRequestHMACSmallJSON(b *testing.B) {
	c := mustNewAPIClient("http://iris.invalid", "secret", WithHTTPClient(&http.Client{}))
	body := []byte(`{"room":"room","type":"text","data":"hello"}`)
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := c.newSignedRequest(ctx, http.MethodPost, PathReply, body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSha256HexBytesEmpty(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = irishmac.SHA256HexBytes(nil)
	}
}
