package webhook

import (
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const (
	PathWebhook = "/webhook/iris"
	// Deprecated: Token-only webhook authentication is unsupported; use signature v2 headers via webhooksign.SignRequest. It will be removed in the next major release.
	HeaderIrisToken            = "X-Iris-Token"
	HeaderIrisMessageID        = irishmac.HeaderIrisMessageID
	HeaderIrisRoute            = "X-Iris-Route"
	HeaderIrisSignatureVersion = irishmac.HeaderIrisSignatureVersion

	SignatureVersionV2 = irishmac.SignatureVersionV2

	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)

const DefaultDedupTTL = 60 * time.Second
