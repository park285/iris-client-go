package signing

import (
	"github.com/park285/iris-client-go/internal/client/randomhex"
	"github.com/park285/iris-client-go/internal/irishmac"
)

func SignIrisCanonicalWithSigner(signer *HMACSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
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

	return signer.Sign(canonical), nil
}

func CanonicalIrisRequest(method, target, timestamp, nonce, bodySHA256 string) string {
	return irishmac.CanonicalRequest(method, target, timestamp, nonce, bodySHA256)
}

func CanonicalIrisTarget(target string) (string, error) {
	return irishmac.CanonicalTarget(target)
}

func GenerateNonce() string {
	return randomhex.Generate("iris-nonce")
}
