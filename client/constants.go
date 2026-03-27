package client

const (
	PathReply               = "/reply"
	PathReplyImage          = "/reply-image"
	PathReplyMarkdown       = "/reply-markdown"
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
	HeaderBotToken      = "X-Bot-Token"      //nolint:gosec // HTTP header name, not a credential
	HeaderIrisTimestamp = "X-Iris-Timestamp"
	HeaderIrisNonce     = "X-Iris-Nonce"
	HeaderIrisSignature = "X-Iris-Signature"
)
