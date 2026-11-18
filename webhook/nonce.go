package webhook

import (
	"context"
	"time"
)

type NonceStore interface {
	IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type SetOnceNonceStore interface {
	NonceStore

	SetOnceNonce()
}
