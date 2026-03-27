package client

const (
	PathReply      = "/reply"
	PathReplyImage = "/reply-image"
	PathReady      = "/ready"
	PathHealth     = "/health"
	PathConfig     = "/config"
	PathDecrypt    = "/decrypt"
)

const HeaderBotToken = "X-Bot-Token" //nolint:gosec // HTTP header name, not a credential
