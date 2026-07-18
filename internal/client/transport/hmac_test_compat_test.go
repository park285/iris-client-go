package transport

import (
	"net/http"

	"github.com/park285/iris-client-go/internal/client/signing"
)

type hmacSigner = signing.HMACSigner

const emptyBodySHA256Hex = signing.EmptyBodySHA256Hex

func newHMACSigner(secret string) *hmacSigner {
	return signing.NewHMACSigner(secret)
}

func signIrisCanonicalWithSigner(signer *hmacSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	return signing.SignIrisCanonicalWithSigner(signer, method, path, timestamp, nonce, bodySHA256)
}

func canonicalIrisRequest(method, target, timestamp, nonce, bodySHA256 string) string {
	return signing.CanonicalIrisRequest(method, target, timestamp, nonce, bodySHA256)
}

func canonicalIrisTarget(target string) (string, error) {
	return signing.CanonicalIrisTarget(target)
}

func generateNonce() string {
	return signing.GenerateNonce()
}

func sha256HexBytes(body []byte) string {
	return signing.SHA256HexBytes(body)
}

func setIrisHMACHeaders(req *http.Request, signer *hmacSigner, method, path, bodySHA256 string) error {
	return signing.SetIrisHMACHeaders(req, signer, method, path, bodySHA256)
}
