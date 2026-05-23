package client

import (
	"strings"
	"unicode"
)

func normalizeReplyThreadID(threadID *string) *string {
	if threadID == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*threadID)
	if trimmed == "" {
		return nil
	}

	for _, r := range trimmed {
		if !unicode.IsDigit(r) {
			return nil
		}
	}

	return &trimmed
}

func normalizeReplyThreadScope(scope *int) *int {
	if scope == nil || *scope <= 0 {
		return nil
	}

	value := *scope

	return &value
}
