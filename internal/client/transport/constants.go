package transport

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
	contentTypeJSON      = "application/json"
	msgTypeText          = "text"
	msgTypeMarkdown      = "markdown"
	msgTypeRichText      = "richtext"
	msgTypeImage         = "image"
	msgTypeImageMultiple = "image_multiple"
)

const mimeImagePNG = "image/png"

// TransportH3와 TransportHTTP1은 WithTransport·ParseTransport의 정본 값이다.
const (
	TransportH3    = "h3"
	TransportHTTP1 = "http1"
)
