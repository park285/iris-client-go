package signing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

func SignIrisCanonicalWithSigner(signer *HMACSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	target, err := irishmac.CanonicalRequestTarget(path)
	if err != nil {
		return "", fmt.Errorf("canonical target: %w", err)
	}

	canonical := irishmac.CanonicalRequest(
		method,
		target,
		timestamp,
		nonce,
		bodySHA256,
	)

	return signer.Sign(canonical), nil
}

func GenerateNonce(epoch string) string {
	var (
		random [32]byte
		nonce  [97]byte
	)

	_, _ = rand.Read(random[:])

	copy(nonce[:32], epoch)

	nonce[32] = '.'
	hex.Encode(nonce[33:], random[:])

	return string(nonce[:])
}
