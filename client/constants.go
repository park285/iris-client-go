package client

const (
	PathReply               = "/reply"
	PathReplyStatus         = "/reply-status"
	PathReady               = "/ready"
	PathHealth              = "/health"
	PathConfig              = "/config"
	PathDiagnosticsBridge   = "/diagnostics/bridge"
	PathDiagnosticsChatroom = "/diagnostics/chatroom-fields"
	PathRooms               = "/rooms"
	PathEventsStream        = "/events/stream"

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
