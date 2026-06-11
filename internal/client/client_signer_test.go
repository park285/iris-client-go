package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSignerForReturnsPrebuiltInstanceForRegisteredSecrets(t *testing.T) {
	t.Parallel()

	c := NewH2CClient("http://iris.invalid", "shared-fallback-token",
		WithTransport("http1"),
		WithInboundSecret("inbound-signing-secret"),
		WithBotControlToken("bot-control-secret"),
		WithHMACSecret("shared-hmac-secret"),
	)

	if len(c.signers) == 0 {
		t.Fatal("client built no prebuilt signers")
	}

	for secret, want := range c.signers {
		if got := c.signerFor(secret); got != want {
			t.Fatalf("signerFor(%q) = %p, want prebuilt %p (hot path must not allocate a fallback signer)", secret, got, want)
		}
	}

	for _, role := range []SecretRole{SecretRoleInbound, SecretRoleBotControl} {
		secret := c.secretFor(role)
		if secret == "" {
			t.Fatalf("secretFor(role=%d) returned empty secret", role)
		}
		prebuilt, ok := c.signers[secret]
		if !ok {
			t.Fatalf("secretFor(role=%d)=%q has no prebuilt signer", role, secret)
		}
		if got := c.signerFor(secret); got != prebuilt {
			t.Fatalf("role=%d signerFor(%q) = %p, want prebuilt %p", role, secret, got, prebuilt)
		}
	}
}

func TestSignerForFallsBackForUnregisteredSecret(t *testing.T) {
	t.Parallel()

	c := NewH2CClient("http://iris.invalid", "shared-fallback-token",
		WithTransport("http1"),
		WithBotControlToken("bot-control-secret"),
	)

	prebuilt := c.signerFor("bot-control-secret")
	if _, ok := c.signers["bot-control-secret"]; !ok {
		t.Fatal("bot-control-secret should be prebuilt")
	}

	fallback := c.signerFor("never-registered-secret")
	if fallback == nil {
		t.Fatal("signerFor(unregistered) = nil, want a fallback signer")
	}
	if fallback == prebuilt {
		t.Fatal("fallback signer must not alias a prebuilt signer")
	}
}

func TestClientSignerMatchesLegacyHelper(t *testing.T) {
	t.Parallel()

	const (
		secret    = "route-secret"
		method    = "POST"
		path      = "/reply"
		timestamp = "1711600000000"
		nonce     = "nonce-legacy-eq"
	)
	bodySHA256 := emptyBodySHA256Hex

	c := NewH2CClient("http://iris.invalid", secret, WithTransport("http1"))
	clientSig, err := signIrisCanonicalWithSigner(c.signerFor(secret), method, path, timestamp, nonce, bodySHA256)
	if err != nil {
		t.Fatalf("client signer signing error = %v", err)
	}

	legacySig := mustSignIrisRequestWithBodySHA256(t, secret, method, path, timestamp, nonce, bodySHA256)

	if clientSig != legacySig {
		t.Fatalf("client signer signature = %q, legacy helper = %q; must match for identical canonical input", clientSig, legacySig)
	}
}

func TestPooledSignerMatchesFreshHMACOverManyIterations(t *testing.T) {
	t.Parallel()

	const secret = "pool-reuse-secret"
	key := []byte(secret)
	signer := newHMACSigner(secret)

	bases := []string{
		"",
		"POST\n/reply\n1711600000000\nnonce-a\n" + emptyBodySHA256Hex,
		"GET\n/rooms\n1711600000001\nnonce-b\nbodyhash-b",
		"POST\n/config\n1711600000002\nnonce-c\nbodyhash-c",
	}

	for i := range 1000 {
		canonical := bases[i%len(bases)]

		got := signer.sign(canonical)

		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(canonical))
		want := hex.EncodeToString(mac.Sum(nil))

		if got != want {
			t.Fatalf("iteration %d: pooled signer.sign(%q) = %q, want fresh hmac %q", i, canonical, got, want)
		}
	}
}
