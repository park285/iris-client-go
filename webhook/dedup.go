package webhook

import (
	"context"
	"time"
)

// Deduplicator checks for duplicate webhook messages.
type Deduplicator interface {
	IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// NoopDeduplicator always reports messages as not duplicate.
type NoopDeduplicator struct{}

func (NoopDeduplicator) IsDuplicate(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, nil
}
