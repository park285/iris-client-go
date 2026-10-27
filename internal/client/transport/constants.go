package transport

import "github.com/park285/iris-client-go/v3/internal/irishmac"

const (
	PathReply       = "/reply"
	PathReplyStatus = "/reply-status"
	PathReady       = "/ready"
	PathHealth      = "/health"
	PathRooms       = "/rooms"

	PathQueryMemberStats    = "/query/member-stats"
	PathQueryRecentMessages = "/query/recent-messages"
)

const (
	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)

const (
	contentTypeJSON      = "application/json"
	msgTypeText          = "text"
	msgTypeMarkdown      = "markdown"
	msgTypeImage         = "image"
	msgTypeImageMultiple = "image_multiple"
)

const (
	mimeApplicationOctetStream = "application/octet-stream"
	mimeImagePNG               = "image/png"
)

const (
	transportH3    = "h3"
	transportHTTP1 = "http1"
)
