package signing

import (
	"net/http"
	"strconv"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const EmptyBodySHA256Hex = irishmac.EmptyBodySHA256Hex

func SHA256HexBytes(body []byte) string {
	return irishmac.SHA256HexBytes(body)
}

func SetIrisHMACHeaders(req *http.Request, signer *HMACSigner, method, path, bodySHA256 string) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := GenerateNonce()
	signature, err := SignIrisCanonicalWithSigner(
		signer,
		method,
		path,
		timestamp,
		nonce,
		bodySHA256,
	)
	if err != nil {
		return err
	}

	req.Header.Set(irishmac.HeaderIrisTimestamp, timestamp)
	req.Header.Set(irishmac.HeaderIrisNonce, nonce)
	req.Header.Set(irishmac.HeaderIrisSignature, signature)
	req.Header.Set(irishmac.HeaderIrisBodySHA256, bodySHA256)
	return nil
}
