package iris

import "time"

const (
	PathWebhook = "/webhook/iris"
	PathReply   = "/reply"
	PathReady   = "/ready"
	PathHealth  = "/health"
	PathConfig  = "/config"
	PathDecrypt = "/decrypt"
)

const (
	HeaderIrisToken     = "X-Iris-Token"
	HeaderIrisMessageID = "X-Iris-Message-Id"
	HeaderBotToken      = "X-Bot-Token" //nolint:gosec // HTTP header name, not a credential
)

const DefaultWebhookDedupTTL = 60 * time.Second
