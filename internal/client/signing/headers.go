package signing

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

func SetIrisHMACHeaders(req *http.Request, signer *irishmac.Signer, method, path, bodySHA256, epoch string) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := GenerateNonce(epoch)

	signature, err := SignIrisCanonicalWithSigner(
		signer,
		method,
		path,
		timestamp,
		nonce,
		bodySHA256,
	)
	if err != nil {
		return fmt.Errorf("sign request: %w", err)
	}

	req.Header.Set(irishmac.HeaderIrisTimestamp, timestamp)
	req.Header.Set(irishmac.HeaderIrisNonce, nonce)
	req.Header.Set(irishmac.HeaderIrisSignature, signature)
	req.Header.Set(irishmac.HeaderIrisBodySHA256, bodySHA256)

	return nil
}
