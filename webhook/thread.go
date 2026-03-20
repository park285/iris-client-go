package webhook

import "strings"

// ResolveThreadID resolves the effective thread ID from a webhook request.
// If ThreadID is present, it's used directly.
// Otherwise, for open-talk rooms (RoomType "OD" or RoomLinkID present),
// ChatLogID is used as a fallback.
func ResolveThreadID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	if id := strings.TrimSpace(req.ThreadID); id != "" {
		return id
	}

	chatLogID := strings.TrimSpace(req.ChatLogID)
	if chatLogID == "" {
		return ""
	}

	roomType := strings.TrimSpace(req.RoomType)

	roomLinkID := strings.TrimSpace(req.RoomLinkID)
	if strings.EqualFold(roomType, "OD") || roomLinkID != "" {
		return chatLogID
	}

	return ""
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
