package iris

import basewebhook "github.com/park285/iris-client-go/webhook"

type WebhookDedupMode = basewebhook.DedupMode

const (
	WebhookDedupModeBeforeDecode = basewebhook.DedupModeBeforeDecode
	WebhookDedupModeAfterDecode  = basewebhook.DedupModeAfterDecode
)

var WithDedupMode = basewebhook.WithDedupMode
