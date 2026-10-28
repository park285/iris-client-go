package transport

import (
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/park285/iris-client-go/v4/internal/irishmac"
)

// authVector는 서버와 공유하는 HMAC 인증 테스트 벡터를 나타냅니다.
type authVector struct {
	Name             string `json:"name"`
	Secret           string `json:"secret"`
	Method           string `json:"method"`
	Target           string `json:"target"`
	TimestampMs      string `json:"timestampMs"`
	Nonce            string `json:"nonce"`
	Body             string `json:"body"`
	BodySha256Hex    string `json:"bodySha256Hex"`
	CanonicalRequest string `json:"canonicalRequest"`
	Signature        string `json:"signature"`
	// ExpectedStatus가 400이면 Iris가 인증 전에 거절하는 target이다. 0은 서명 검증까지 통과한다.
	ExpectedStatus int `json:"expectedStatus"`
}

// TestSignIrisRequestContractVectors는 서버 측 인증 벡터와 SDK 서명 로직의 호환성을 검증합니다.
// 벡터 파일: testdata/iris_auth_vectors.json (Iris 서버와 동일한 테스트 데이터).
func TestSignIrisRequestContractVectors(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/iris_auth_vectors.json")
	if err != nil {
		t.Fatalf("벡터 파일 읽기 실패: %v", err)
	}

	var vectors []authVector

	if err := jsonv2.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("벡터 파일 파싱 실패: %v", err)
	}

	if len(vectors) == 0 {
		t.Fatal("벡터가 비어 있습니다")
	}

	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			t.Parallel()

			// 본문 SHA-256 해시 검증
			bodyHash := sha256.Sum256([]byte(v.Body))
			gotBodyHash := hex.EncodeToString(bodyHash[:])

			if gotBodyHash != v.BodySha256Hex {
				t.Errorf("body sha256 불일치: got %s, want %s", gotBodyHash, v.BodySha256Hex)
			}

			switch v.ExpectedStatus {
			case 0:
			case http.StatusBadRequest:
				// Iris가 400으로 거절할 target은 SDK가 서명하지 않고 요청 전에 실패한다.
				_, err := signIrisRequest(v.Secret, v.Method, v.Target, v.TimestampMs, v.Nonce, v.Body)
				if !errors.Is(err, irishmac.ErrDuplicateQueryKey) {
					t.Fatalf("signIrisRequest(%q) error = %v, want %v", v.Target, err, irishmac.ErrDuplicateQueryKey)
				}

				return
			default:
				t.Fatalf("해석하지 않는 expectedStatus %d", v.ExpectedStatus)
			}

			gotCanonicalRequest := irishmac.CanonicalRequest(
				v.Method,
				mustCanonicalIrisTarget(t, v.Target),
				v.TimestampMs,
				v.Nonce,
				gotBodyHash)

			if gotCanonicalRequest != v.CanonicalRequest {
				t.Errorf("canonical request 불일치:\n  got:  %s\n  want: %s", gotCanonicalRequest, v.CanonicalRequest)
			}

			// HMAC 서명 검증
			got := mustSignIrisRequest(t, v.Secret, v.Method, v.Target, v.TimestampMs, v.Nonce, v.Body)
			if got != v.Signature {
				t.Errorf("서명 불일치:\n  got:  %s\n  want: %s", got, v.Signature)
			}
		})
	}
}
