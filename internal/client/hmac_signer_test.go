package client

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
		signer := newHMACSigner(secret)
		for _, body := range bodies {
			got := signer.sign(body)

			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(body))
			want := hex.EncodeToString(mac.Sum(nil))

			if got != want {
				t.Fatalf("signer.sign(secret=%q, body=%q) = %q, want %q", secret, body, got, want)
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

	signer := newHMACSigner(secret)
	pooled := testing.AllocsPerRun(1000, func() {
		_ = signer.sign(canonical)
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

	const secret = "concurrent-secret"
	signer := newHMACSigner(secret)
	canonical := "POST\n/reply\n1711600000000\nnonce-c\nbodyhash"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))

	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				if got := signer.sign(canonical); got != want {
					t.Errorf("concurrent signer.sign = %q, want %q", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

const (
	benchSignSecret    = "bench-sign-secret"
	benchSignMethod    = http.MethodPost
	benchSignPath      = PathReply
	benchSignTimestamp = "1711600000000"
	benchSignNonce     = "bench-nonce"
)

func BenchmarkSignIrisRequestLegacyHelper(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := signIrisRequestWithBodySHA256(
			benchSignSecret, benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, emptyBodySHA256Hex,
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignIrisRequestClientSigner(b *testing.B) {
	signer := newHMACSigner(benchSignSecret)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := signIrisCanonicalWithSigner(
			signer, benchSignMethod, benchSignPath, benchSignTimestamp, benchSignNonce, emptyBodySHA256Hex,
		); err != nil {
			b.Fatal(err)
		}
	}
}
