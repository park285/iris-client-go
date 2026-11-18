package irishmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

func TestSignerSignSurvivesForeignPoolValue(t *testing.T) {
	t.Parallel()

	const secret = "pool-poison-secret" // #nosec G101 -- 테스트 픽스처 값이다.

	signer := NewSigner(secret)
	canonical := "POST\n/reply\n1711600000000\nnonce-p\nbodyhash"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))

	want := hex.EncodeToString(mac.Sum(nil))

	signer.pool.Put(new(string))

	if got := signer.Sign(canonical); got != want {
		t.Fatalf("Sign() after foreign pool value = %q, want %q", got, want)
	}
}

func TestCanonicalWebhookRequestV3NormalizesAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		authority string
		want      string
	}{
		{name: "hostname", authority: "Webhook.Example:08443", want: "webhook.example:8443"},
		{name: "IPv4", authority: "192.0.2.10", want: "192.0.2.10"},
		{name: "IPv6", authority: "[2001:0DB8:0:0:0:0:0:1]:443", want: "[2001:db8::1]:443"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			canonical, err := CanonicalWebhookRequestV3(
				test.authority,
				"post",
				"/webhook/iris",
				"9003",
				"nonce-v3",
				"message-v3",
				strings.ToUpper(EmptyBodySHA256Hex),
			)
			if err != nil {
				t.Fatalf("CanonicalWebhookRequestV3() error = %v", err)
			}

			want := strings.Join([]string{
				SignatureVersionV3,
				test.want,
				"POST",
				"/webhook/iris",
				"9003",
				"nonce-v3",
				"message-v3",
				EmptyBodySHA256Hex,
			}, "\n")
			if canonical != want {
				t.Fatalf("CanonicalWebhookRequestV3() = %q, want %q", canonical, want)
			}
		})
	}
}

func TestCanonicalWebhookRequestV3RejectsInvalidAuthority(t *testing.T) {
	t.Parallel()

	for _, authority := range []string{
		"",
		" webhook.example",
		"user@webhook.example",
		"webhook.example/path",
		"webhook.example:",
		"webhook.example:65536",
		"2001:db8::1",
		"[192.0.2.10]",
		"웹훅.example",
	} {
		t.Run(authority, func(t *testing.T) {
			t.Parallel()

			if _, err := CanonicalWebhookRequestV3(
				authority,
				"POST",
				"/webhook/iris",
				"9003",
				"nonce-v3",
				"message-v3",
				EmptyBodySHA256Hex,
			); err == nil {
				t.Fatalf("CanonicalWebhookRequestV3(%q) error = nil", authority)
			}
		})
	}
}

func TestHMACSignerMatchesCryptoHMAC(t *testing.T) {
	t.Parallel()

	secrets := []string{"secret-a", "secret-b", "", "another longer shared secret value"}
	bodies := []string{
		"",
		`{"room":"r","type":"text","data":"hello"}`,
		"POST\n/reply\n1711600000000\nnonce\nabc",
	}

	for _, secret := range secrets {
		signer := NewSigner(secret)

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

	signer := NewSigner(secret)
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

	signer := NewSigner(secret)
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
