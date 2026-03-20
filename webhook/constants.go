package webhook

import "time"

const (
	PathWebhook         = "/webhook/iris"
	HeaderIrisToken     = "X-Iris-Token"
	HeaderIrisMessageID = "X-Iris-Message-Id"
)

const DefaultDedupTTL = 60 * time.Second
