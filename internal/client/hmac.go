package client

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

var randomHexFallbackCounter atomic.Uint64

func signIrisCanonicalWithSigner(signer *hmacSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	target, err := irishmac.CanonicalTarget(path)
	if err != nil {
		return "", err
	}

	canonical := irishmac.CanonicalRequest(
		method,
		target,
		timestamp,
		nonce,
		bodySHA256,
	)

	return signer.sign(canonical), nil
}

func canonicalIrisRequest(method, target, timestamp, nonce, bodySHA256 string) string {
	return irishmac.CanonicalRequest(method, target, timestamp, nonce, bodySHA256)
}

func canonicalIrisTarget(target string) (string, error) {
	return irishmac.CanonicalTarget(target)
}

func generateNonce() string {
	return generateRandomHex16("iris-nonce")
}

func generateRandomHex16(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}

	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), randomHexFallbackCounter.Add(1))
}
