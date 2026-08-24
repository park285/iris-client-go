package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

// per-call newHMACSigner 생성은 test helper에만 허용한다.
func signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	bodyHash := sha256.Sum256([]byte(body))
	return signIrisRequestWithBodySHA256(secret, method, path, timestamp, nonce, hex.EncodeToString(bodyHash[:])) //nolint:wrapcheck // 테스트 헬퍼는 서명 오류를 그대로 돌려 검사한다.
}

func signIrisRequestWithBodySHA256(secret, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	return signing.SignIrisCanonicalWithSigner(signing.NewHMACSigner(secret), method, path, timestamp, nonce, bodySHA256) //nolint:wrapcheck // 테스트 헬퍼는 서명 오류를 그대로 돌려 검사한다.
}

func mustCanonicalIrisTarget(tb testing.TB, target string) string {
	tb.Helper()

	got, err := signing.CanonicalIrisTarget(target)
	if err != nil {
		tb.Fatalf("canonicalIrisTarget(%q) error = %v", target, err)
	}

	return got
}

func mustSignIrisRequest(tb testing.TB, secret, method, path, timestamp, nonce, body string) string {
	tb.Helper()

	sig, err := signIrisRequest(secret, method, path, timestamp, nonce, body)
	if err != nil {
		tb.Fatalf("signIrisRequest(%q) error = %v", path, err)
	}

	return sig
}

func mustSignIrisRequestWithBodySHA256(tb testing.TB, secret, method, path, timestamp, nonce, bodySHA256 string) string {
	tb.Helper()

	sig, err := signIrisRequestWithBodySHA256(secret, method, path, timestamp, nonce, bodySHA256)
	if err != nil {
		tb.Fatalf("signIrisRequestWithBodySHA256(%q) error = %v", path, err)
	}

	return sig
}
