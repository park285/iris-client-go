package transport

import "github.com/park285/iris-client-go/v4/internal/irishmac"

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

const mimeImagePNG = "image/png"

// TransportH3와 TransportHTTP1은 WithTransport·ParseTransport의 정본 값이다.
const (
	TransportH3    = "h3"
	TransportHTTP1 = "http1"
)
