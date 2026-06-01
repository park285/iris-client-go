package client

const (
	PathReply                   = "/reply"
	PathReplyStatus             = "/reply-status"
	PathReady                   = "/ready"
	PathHealth                  = "/health"
	PathConfig                  = "/config"
	PathDiagnosticsBridge       = "/diagnostics/bridge"
	PathDiagnosticsChatroom     = "/diagnostics/chatroom-fields"
	PathDiagnosticsNativeCore   = "/diagnostics/native-core"
	PathDiagnosticsRuntime      = "/diagnostics/runtime"
	PathDiagnosticsTextPing     = "/diagnostics/text-ping"
	PathDiagnosticsChatroomOpen = "/diagnostics/chatroom-open"
	PathKaringSend              = "/karing/send"
	PathKaringContentList       = "/karing/content-list"
	PathKaringHololive          = "/karing/hololive"
	PathRooms                   = "/rooms"
	PathEventsStream            = "/events/stream"

	PathQueryRoomSummary    = "/query/room-summary"
	PathQueryMemberStats    = "/query/member-stats"
	PathQueryRecentThreads  = "/query/recent-threads"
	PathQueryRecentMessages = "/query/recent-messages"
)

const (
	HeaderIrisTimestamp  = "X-Iris-Timestamp"
	HeaderIrisNonce      = "X-Iris-Nonce"
	HeaderIrisSignature  = "X-Iris-Signature"
	HeaderIrisBodySHA256 = "X-Iris-Body-Sha256"
)

const (
	msgTypeText          = "text"
	msgTypeImage         = "image"
	msgTypeImageMultiple = "image_multiple"
)

const mimeImagePNG = "image/png"

const (
	transportH3    = "h3"
	transportH2C   = "h2c"
	transportHTTP2 = "http2"
	transportHTTP1 = "http1"
)
