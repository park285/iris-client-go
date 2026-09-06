package iris_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/park285/iris-client-go/v2/iris"
)

func TestTypedJSONLimitUsesDecompressedBytesThroughPublicClient(t *testing.T) {
	const limit = 16 << 20

	for _, size := range []int{limit, limit + 1} {
		compressed := gzipResponseFixture(t, size)
		if len(compressed) >= limit {
			t.Fatal("fixture must be small on the wire")
		}

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(fmt.Sprintf("%s/%d", method, size), func(t *testing.T) {
				var attempts atomic.Int32

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					attempts.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Content-Encoding", "gzip")

					if _, err := w.Write(compressed); err != nil {
						t.Errorf("write compressed fixture: %v", err)
					}
				}))
				t.Cleanup(server.Close)

				client := iris.NewAPIClient(server.URL, "synthetic-test-token", iris.WithHTTPClient(server.Client()), iris.WithReplyRetry(3))

				var err error

				if method == http.MethodGet {
					_, err = client.GetReplyStatus(t.Context(), "test")
				} else {
					_, err = client.SendMessageAccepted(t.Context(), "test-room", "test", iris.WithClientRequestID("gzip-limit-test"))
				}

				assertPublicResponseLimit(t, method, size > limit, err)

				if attempts.Load() != 1 {
					t.Fatalf("attempts=%d, want one", attempts.Load())
				}
			})
		}
	}
}

func assertPublicResponseLimit(t *testing.T, method string, tooLarge bool, err error) {
	t.Helper()

	if !tooLarge {
		if err != nil {
			t.Fatalf("exact decompressed boundary failed: %v", err)
		}

		return
	}

	if !errors.Is(err, iris.ErrResponseTooLarge) || errors.Is(err, iris.ErrRetryable) {
		t.Fatalf("oversized gzip error=%v", err)
	}

	if method == http.MethodPost && !errors.Is(err, iris.ErrTransport) {
		t.Fatalf("POST lost outcome unknown: %v", err)
	}
}

func gzipResponseFixture(t *testing.T, size int) []byte {
	t.Helper()

	var compressed bytes.Buffer

	writer := gzip.NewWriter(&compressed)

	const (
		prefix = `{"requestId":"test","room":"`
		suffix = `"}`
	)

	if _, err := writer.Write([]byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return compressed.Bytes()
}
