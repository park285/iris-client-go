package webhook

import "strings"

func resolveThreadID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	return strings.TrimSpace(req.ThreadID)
}
