package client

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

const emptyBodySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func sha256HexBytes(body []byte) string {
	if len(body) == 0 {
		return emptyBodySHA256Hex
	}

	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func setIrisHMACHeaders(req *http.Request, secret, method, path, bodySHA256 string) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := generateNonce()
	signature := signIrisRequestWithBodySHA256(
		secret,
		method,
		path,
		timestamp,
		nonce,
		bodySHA256,
	)

	req.Header.Set(HeaderIrisTimestamp, timestamp)
	req.Header.Set(HeaderIrisNonce, nonce)
	req.Header.Set(HeaderIrisSignature, signature)
	req.Header.Set(HeaderIrisBodySHA256, bodySHA256)
}
