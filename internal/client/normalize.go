package client

import (
	"fmt"
	"strings"
)

func normalizeReplyThreadID(threadID *string) *string {
	if threadID == nil {
		return nil
	}

	normalized, err := normalizeReplyThreadIDValue(*threadID)
	if err != nil {
		return nil
	}

	return &normalized
}

func normalizeReplyThreadIDValue(threadID string) (string, error) {
	trimmed := strings.TrimSpace(threadID)
	if trimmed == "" {
		return "", fmt.Errorf("iris: threadId must not be blank")
	}

	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < '0' || trimmed[i] > '9' {
			return "", fmt.Errorf("iris: threadId must be numeric, got %q", threadID)
		}
	}

	return trimmed, nil
}

func normalizeReplyThreadScope(scope *int) *int {
	if scope == nil || *scope <= 0 {
		return nil
	}

	value := *scope

	return &value
}
