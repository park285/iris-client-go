package irishmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// Sign은 pool에서 꺼낸 값의 타입 단언이 실패해도 새 MAC으로 진행해야 한다. 그러지 않으면
// 다른 코드가 같은 pool에 이물질을 넣었을 때 서명이 조용히 깨진다.
func TestSignerSignSurvivesForeignPoolValue(t *testing.T) {
	t.Parallel()

	const secret = "pool-poison-secret"
	signer := NewSigner(secret)
	canonical := "POST\n/reply\n1711600000000\nnonce-p\nbodyhash"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))

	signer.pool.Put(new(string))
	if got := signer.Sign(canonical); got != want {
		t.Fatalf("Sign() after foreign pool value = %q, want %q", got, want)
	}
}
