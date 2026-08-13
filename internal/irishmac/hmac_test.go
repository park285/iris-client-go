package irishmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// Sign은 pool에서 꺼낸 값의 타입 단언이 실패해도 새 MAC으로 진행해야 한다. 그러지 않으면
// 다른 코드가 같은 pool에 이물질을 넣었을 때 서명이 조용히 깨진다.
func TestSignerSignSurvivesForeignPoolValue(t *testing.T) {
	t.Parallel()

	const secret = "pool-poison-secret"
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
