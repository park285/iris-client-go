package signing

import (
	"net/http"
	"testing"

	"github.com/park285/iris-client-go/v6/internal/irishmac"
)

const (
	benchSignSecret    = "bench-sign-secret" // #nosec G101 -- 벤치마크 픽스처 값이다.
	benchSignMethod    = http.MethodPost
	benchSignPath      = PathReply
	benchSignTimestamp = "1711600000000"
	benchSignNonce     = "bench-nonce"
)

func BenchmarkSignIrisRequestPerCallSigner(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := SignIrisCanonicalWithSigner(irishmac.NewSigner(
			benchSignSecret),
			benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, irishmac.EmptyBodySHA256Hex); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignIrisRequestClientSigner(b *testing.B) {
	signer := irishmac.NewSigner(benchSignSecret)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := SignIrisCanonicalWithSigner(
			signer, benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, irishmac.EmptyBodySHA256Hex); err != nil {
			b.Fatal(err)
		}
	}
}
