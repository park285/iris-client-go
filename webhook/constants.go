package webhook

import (
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const (
	PathWebhook                = "/webhook/iris"
	HeaderIrisToken            = "X-Iris-Token"
	HeaderIrisMessageID        = "X-Iris-Message-Id"
	HeaderIrisRoute            = "X-Iris-Route"
	HeaderIrisSignatureVersion = "X-Iris-Signature-Version"

	SignatureVersionV1 = "v1"
	SignatureVersionV2 = "v2"

	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)

const DefaultDedupTTL = 60 * time.Second
