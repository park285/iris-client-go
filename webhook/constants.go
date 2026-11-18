package webhook

import "github.com/park285/iris-client-go/v5/internal/irishmac"

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

const (
	MessageTypeText       = "1"
	MessageTypePhoto      = "2"
	MessageTypeFile       = "18"
	MessageTypeReply      = "26"
	MessageTypeMultiPhoto = "27"
)

const (
	RoomTypeDirectChat     = "DirectChat"
	RoomTypeMultiChat      = "MultiChat"
	OpenChatRoomTypePrefix = "O"
)

const (
	MaxReferencedImages        = 10
	MaxReferencedImageURLBytes = 2048
)
