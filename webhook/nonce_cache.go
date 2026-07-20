package webhook

import (
	"context"
	"sync"
	"time"
)

type memoryNonceCache struct {
	mu        sync.Mutex
	entries   map[string]time.Time
	lastSweep time.Time
	now       func() time.Time
}

func newMemoryNonceCache() *memoryNonceCache {
	return &memoryNonceCache{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

func (c *memoryNonceCache) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteExpired(now, ttl)
	if expiresAt, ok := c.entries[key]; ok {
		if expiresAt.After(now) {
			return true, nil
		}
		delete(c.entries, key)
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

func (c *memoryNonceCache) deleteExpired(now time.Time, ttl time.Duration) {
	interval := ttl / 4
	if interval > 0 && now.Sub(c.lastSweep) < interval {
		return
	}

	for key, expiresAt := range c.entries {
		if !expiresAt.After(now) {
			delete(c.entries, key)
		}
	}
	c.lastSweep = now
}
