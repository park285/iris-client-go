package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"testing"
)

func TestHMACSignerMatchesCryptoHMAC(t *testing.T) {
	t.Parallel()

	secrets := []string{"secret-a", "secret-b", "", "another longer shared secret value"}
	bodies := []string{
		"",
		`{"room":"r","type":"text","data":"hello"}`,
		"POST\n/reply\n1711600000000\nnonce\nabc",
	}

	for _, secret := range secrets {
		signer := NewHMACSigner(secret)

		for _, body := range bodies {
			got := signer.Sign(body)

			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(body))

			want := hex.EncodeToString(mac.Sum(nil))

			if got != want {
				t.Fatalf("signer.Sign(secret=%q, body=%q) = %q, want %q", secret, body, got, want)
			}
		}
	}
}

func TestHMACSignerReusesKeySchedule(t *testing.T) {
	const secret = "reuse-secret"

	canonical := "POST\n/reply\n1711600000000\nnonce-xyz\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	fresh := testing.AllocsPerRun(1000, func() {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(canonical))

		_ = hex.EncodeToString(mac.Sum(nil))
	})

	signer := NewHMACSigner(secret)
	pooled := testing.AllocsPerRun(1000, func() {
		_ = signer.Sign(canonical)
	})

	if pooled >= fresh {
		t.Fatalf("pooled signer allocs/run = %f, fresh hmac.New = %f; key schedule must be reused (pooled must be strictly fewer)", pooled, fresh)
	}

	if !raceEnabled && pooled > 4 {
		t.Fatalf("signer.sign allocs/run = %f, want <= 4 (key schedule reuse leaves only call-boundary escapes)", pooled)
	}
}

func TestHMACSignerConcurrentSign(t *testing.T) {
	t.Parallel()

	const secret = "concurrent-secret" // #nosec G101 -- 테스트 픽스처 값이다.

	signer := NewHMACSigner(secret)
	canonical := "POST\n/reply\n1711600000000\nnonce-c\nbodyhash"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))

	want := hex.EncodeToString(mac.Sum(nil))

	var wg sync.WaitGroup

	for range 64 {
		wg.Go(func() {
			for range 200 {
				if got := signer.Sign(canonical); got != want {
					t.Errorf("concurrent signer.sign = %q, want %q", got, want)

					return
				}
			}
		})
	}

	wg.Wait()
}

const (
	benchSignSecret    = "bench-sign-secret" // #nosec G101 -- 벤치마크 픽스처 값이다.
	benchSignMethod    = http.MethodPost
	benchSignPath      = PathReply
	benchSignTimestamp = "1711600000000"
	benchSignNonce     = "bench-nonce"
)

func BenchmarkSignIrisRequestLegacyHelper(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := SignIrisCanonicalWithSigner(NewHMACSigner(
			benchSignSecret),
			benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, EmptyBodySHA256Hex); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignIrisRequestClientSigner(b *testing.B) {
	signer := NewHMACSigner(benchSignSecret)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := SignIrisCanonicalWithSigner(
			signer, benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, EmptyBodySHA256Hex); err != nil {
			b.Fatal(err)
		}
	}
}
