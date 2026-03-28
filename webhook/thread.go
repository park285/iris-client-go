package webhook

import "strings"

func ResolveThreadID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	return strings.TrimSpace(req.ThreadID)
}

// DedupKey는 주어진 메시지 ID로 중복 제거 키를 생성합니다.
// messageID가 비어 있으면 빈 문자열을 반환합니다.
func DedupKey(messageID string) string {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return ""
	}

	return "iris:msg:{" + id + "}"
}
