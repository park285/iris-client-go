package client

import (
	"net/http"
	"strconv"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const emptyBodySHA256Hex = irishmac.EmptyBodySHA256Hex

func sha256HexBytes(body []byte) string {
	return irishmac.SHA256HexBytes(body)
}

func setIrisHMACHeaders(req *http.Request, signer *hmacSigner, method, path, bodySHA256 string) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := generateNonce()
	signature, err := signIrisCanonicalWithSigner(
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

	req.Header.Set(HeaderIrisTimestamp, timestamp)
	req.Header.Set(HeaderIrisNonce, nonce)
	req.Header.Set(HeaderIrisSignature, signature)
	req.Header.Set(HeaderIrisBodySHA256, bodySHA256)
	return nil
}
