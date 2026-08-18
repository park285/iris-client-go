package signing

import (
	"github.com/park285/iris-client-go/v2/internal/irishmac"
)

// HMAC 서명 구현은 irishmac이 단독으로 소유한다. 여기에 축자 복제본을 두면 boundary gate가
// 검사하지 못하는 두 번째 구현이 생겨, gate를 통과한 채로 두 구현이 갈라질 수 있다.
// 그래서 이 파일은 타입 별칭과 생성자 위임만 남긴다.
type HMACSigner = irishmac.Signer

func NewHMACSigner(secret string) *HMACSigner {
	return irishmac.NewSigner(secret)
}
