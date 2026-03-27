package webhook

import "strings"

// ResolveThreadID returns the observed thread ID from a webhook request.
func ResolveThreadID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	return strings.TrimSpace(req.ThreadID)
}

// DedupKey generates a deduplication key for a given message ID.
// Returns empty string if messageID is empty.
func DedupKey(messageID string) string {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return ""
	}

	return "iris:msg:{" + id + "}"
}
