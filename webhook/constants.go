package webhook

import "github.com/park285/iris-client-go/v3/internal/irishmac"

const (
	PathWebhook                = "/webhook/iris"
	HeaderIrisMessageID        = irishmac.HeaderIrisMessageID
	HeaderIrisSignatureVersion = irishmac.HeaderIrisSignatureVersion

	SignatureVersionV3 = irishmac.SignatureVersionV3

	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)
