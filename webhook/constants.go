package webhook

import (
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const (
	PathWebhook                = "/webhook/iris"
	HeaderIrisMessageID        = irishmac.HeaderIrisMessageID
	HeaderIrisRoute            = "X-Iris-Route"
	HeaderIrisSignatureVersion = irishmac.HeaderIrisSignatureVersion

	SignatureVersionV2 = irishmac.SignatureVersionV2

	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)

// DefaultDedupTTL은 확정(commit)된 키의 TTL입니다. 이 값이 발신자의 마지막 재전송보다 먼저
// 만료되면 이미 처리한 메시지가 다시 처리되므로, WithDedupTTL로 낮출 때 그 도달 시각을 함께
// 확인하십시오.
const DefaultDedupTTL = 16 * time.Minute
