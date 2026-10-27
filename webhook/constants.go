package webhook

import "github.com/park285/iris-client-go/v4/internal/irishmac"

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

// WebhookRequest.Type으로 오는 KakaoTalk 메시지 type 코드다. 값의 정본은 Iris webhook payload와
// 참조 context 코드(native/iris-runtime/src/webhook/payload, src/ingress/poller/reference_images)다.
const (
	MessageTypeText       = "1"
	MessageTypePhoto      = "2"
	MessageTypeFile       = "18"
	MessageTypeReply      = "26"
	MessageTypeMultiPhoto = "27"
)

// WebhookRequest.RoomType 값이다. DirectChat·MultiChat은 Iris read model이 판정하는 값이고, 오픈채팅
// room type은 OpenChatRoomTypePrefix로 시작한다(예: OD, OM).
const (
	RoomTypeDirectChat     = "DirectChat"
	RoomTypeMultiChat      = "MultiChat"
	OpenChatRoomTypePrefix = "O"
)

// Iris가 attachment의 참조 context(src_images)에 싣는 이미지 URL 상한이다. 값의 정본은 Iris
// native/iris-runtime/src/referenced_context/images.rs의 MAX_REFERENCED_IMAGES·
// MAX_REFERENCED_IMAGE_URL_BYTES다.
const (
	MaxReferencedImages        = 10
	MaxReferencedImageURLBytes = 2048
)
