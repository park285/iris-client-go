package webhook

import (
	"context"
	"sync"
	"time"
)

type memoryNonceCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func newMemoryNonceCache() *memoryNonceCache {
	return &memoryNonceCache{entries: make(map[string]time.Time)}
}

func (c *memoryNonceCache) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteExpired(now)
	if expiresAt, ok := c.entries[key]; ok && expiresAt.After(now) {
		return true, nil
	}

	c.entries[key] = now.Add(ttl)
	return false, nil
}

func (c *memoryNonceCache) Release(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)

	return nil
}

func (c *memoryNonceCache) deleteExpired(now time.Time) {
	for key, expiresAt := range c.entries {
		if !expiresAt.After(now) {
			delete(c.entries, key)
		}
	}
}
