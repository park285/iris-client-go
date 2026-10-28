package signing

import (
	"fmt"

	"github.com/park285/iris-client-go/v4/internal/client/randomhex"
	"github.com/park285/iris-client-go/v4/internal/irishmac"
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

func GenerateNonce() string {
	return randomhex.Generate()
}
