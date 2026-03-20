package iris

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type SendOption func(*sendOptions)

type sendOptions struct {
	ThreadID    *string
	ThreadScope *int
}

func WithThreadID(id string) SendOption {
	return func(o *sendOptions) {
		o.ThreadID = &id
	}
}

func WithThreadScope(scope int) SendOption {
	return func(o *sendOptions) {
		o.ThreadScope = &scope
	}
}

//nolint:revive // Unexported return is intentional; callers access exported helpers around these fields.
func ApplySendOptions(opts []SendOption) sendOptions {
	var result sendOptions

	for _, opt := range opts {
		opt(&result)
	}

	return result
}

// ValidateSendOptions checks threadId/threadScope constraints.
// ThreadScope >= 2 requires threadId (Iris server returns 400 otherwise).
// ThreadId must be numeric-only (Iris server contract).
func ValidateSendOptions(o sendOptions) error {
	if o.ThreadID != nil {
		for _, r := range *o.ThreadID {
			if !unicode.IsDigit(r) {
				return fmt.Errorf("iris: threadId must be numeric, got %q", *o.ThreadID)
			}
		}
	}

	if o.ThreadScope != nil && *o.ThreadScope <= 0 {
		return fmt.Errorf("iris: threadScope must be positive, got %d", *o.ThreadScope)
	}

	if o.ThreadScope != nil && *o.ThreadScope >= 2 && o.ThreadID == nil {
		return errors.New("iris: threadScope >= 2 requires threadId")
	}

	return nil
}

// NormalizeReplyThreadID trims whitespace and returns nil if empty or non-numeric.
func NormalizeReplyThreadID(threadID *string) *string {
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

// NormalizeReplyThreadScope returns nil if scope is nil or non-positive.
func NormalizeReplyThreadScope(scope *int) *int {
	if scope == nil || *scope <= 0 {
		return nil
	}

	value := *scope

	return &value
}
