package signing

import (
	"fmt"

	"github.com/park285/iris-client-go/v2/internal/client/randomhex"
	"github.com/park285/iris-client-go/v2/internal/irishmac"
)

func SignIrisCanonicalWithSigner(signer *HMACSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	target, err := irishmac.CanonicalTarget(path)
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

func CanonicalIrisRequest(method, target, timestamp, nonce, bodySHA256 string) string {
	return irishmac.CanonicalRequest(method, target, timestamp, nonce, bodySHA256)
}

func CanonicalIrisTarget(target string) (string, error) {
	return irishmac.CanonicalTarget(target) //nolint:wrapcheck // irishmac 오류가 대상 맥락을 이미 담고 있다.
}

func GenerateNonce() string {
	return randomhex.Generate()
}
