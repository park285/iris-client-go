package client

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// signIrisRequest는 Iris 요청 인증을 위한 HMAC-SHA256 서명을 계산합니다.
// 정규화 형식: "METHOD\nPATH\nTIMESTAMP\nNONCE\nSHA256(body)"
func signIrisRequest(secret, method, path, timestamp, nonce, body string) string {
	bodyHash := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{
		strings.ToUpper(method),
		path,
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
