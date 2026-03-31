package client

const (
	PathReply               = "/reply"
	PathReplyStatus         = "/reply-status"
	PathReady               = "/ready"
	PathHealth              = "/health"
	PathConfig              = "/config"
	PathDecrypt             = "/decrypt"
	PathQuery               = "/query"
	PathDiagnosticsBridge   = "/diagnostics/bridge"
	PathDiagnosticsChatroom = "/diagnostics/chatroom-fields"
	PathRooms               = "/rooms"
	PathEventsStream        = "/events/stream"
)

const (
	HeaderIrisTimestamp = "X-Iris-Timestamp"
	HeaderIrisNonce     = "X-Iris-Nonce"
	HeaderIrisSignature = "X-Iris-Signature"
	HeaderIrisBodySHA256 = "X-Iris-Body-Sha256"
)
