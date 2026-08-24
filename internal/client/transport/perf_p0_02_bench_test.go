package transport

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

const (
	benchP0HMACSecret = "bench-p0-hmac-secret" // #nosec G101 -- 벤치마크 픽스처 값이다.
	benchP0BodySHA256 = signing.EmptyBodySHA256Hex
)

func BenchmarkSetIrisHMACHeaders(b *testing.B) {
	signer := signing.NewHMACSigner(benchP0HMACSecret)

	req, err := http.NewRequestWithContext(b.Context(), http.MethodPost, "http://iris.invalid"+PathReply, http.NoBody)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := signing.SetIrisHMACHeaders(req, signer, http.MethodPost, PathReply, benchP0BodySHA256); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRetryDelayForPlainError(b *testing.B) {
	orig := halfJitterFloat64

	b.Cleanup(func() { halfJitterFloat64 = orig })

	src := rand.New(rand.NewPCG(0x9E3779B97F4A7C15, 0xBF58476D1CE4E5B9)) //nolint:gosec // 벤치마크 재현성을 위한 고정 시드 PCG.

	halfJitterFloat64 = src.Float64

	plain := errors.New("transport blip")

	const fallback = 200 * time.Millisecond

	b.ReportAllocs()

	for b.Loop() {
		_ = retryDelayForError(plain, fallback)
	}
}

func BenchmarkRetryDelayForRetryAfter(b *testing.B) {
	retryAfter := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: 2 * time.Second})

	const fallback = 200 * time.Millisecond

	b.ReportAllocs()

	for b.Loop() {
		_ = retryDelayForError(retryAfter, fallback)
	}
}

func BenchmarkCanonicalIrisTargetSimple(b *testing.B) {
	const target = PathReply

	b.ReportAllocs()

	for b.Loop() {
		if _, err := signing.CanonicalIrisTarget(target); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanonicalIrisTargetWithQuery(b *testing.B) {
	const target = PathReply + "?room=general&limit=50&cursor=abc123&order=desc"

	b.ReportAllocs()

	for b.Loop() {
		if _, err := signing.CanonicalIrisTarget(target); err != nil {
			b.Fatal(err)
		}
	}
}
