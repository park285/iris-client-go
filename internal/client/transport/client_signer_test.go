package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/park285/iris-client-go/v3/internal/client/signing"
)

func TestClientSignerMatchesLegacyHelper(t *testing.T) {
	t.Parallel()

	const (
		secret    = "route-secret"
		method    = "POST"
		path      = "/reply"
		timestamp = "1711600000000"
		nonce     = "nonce-legacy-eq"
	)

	bodySHA256 := signing.EmptyBodySHA256Hex

	c := NewAPIClient("http://iris.invalid", secret, WithTransport(transportHTTP1))

	clientSig, err := signing.SignIrisCanonicalWithSigner(c.signer, method, path, timestamp, nonce, bodySHA256)
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

	const secret = "pool-reuse-secret" // #nosec G101 -- 테스트 픽스처 값이다.

	key := []byte(secret)
	signer := signing.NewHMACSigner(secret)

	bases := []string{
		"",
		"POST\n/reply\n1711600000000\nnonce-a\n" + signing.EmptyBodySHA256Hex,
		"GET\n/rooms\n1711600000001\nnonce-b\nbodyhash-b",
		"POST\n/config\n1711600000002\nnonce-c\nbodyhash-c",
	}

	for i := range 1000 {
		canonical := bases[i%len(bases)]

		got := signer.Sign(canonical)

		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(canonical))

		want := hex.EncodeToString(mac.Sum(nil))

		if got != want {
			t.Fatalf("iteration %d: pooled signer.sign(%q) = %q, want fresh hmac %q", i, canonical, got, want)
		}
	}
}
