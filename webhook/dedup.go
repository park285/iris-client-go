package webhook

import (
	"context"
	"time"
)

// Deduplicator는 중복 webhook 메시지를 검사하는 인터페이스입니다.
type Deduplicator interface {
	IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type NoopDeduplicator struct{}

func (NoopDeduplicator) IsDuplicate(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, nil
}
