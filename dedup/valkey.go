package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"

	"park285/iris-client-go/webhook"
)

// ValkeyDeduplicator implements webhook.Deduplicator using Valkey SET NX.
type ValkeyDeduplicator struct {
	client valkey.Client
}

// Verify interface compliance.
var _ webhook.Deduplicator = (*ValkeyDeduplicator)(nil)

// NewValkeyDeduplicator creates a new ValkeyDeduplicator.
func NewValkeyDeduplicator(client valkey.Client) *ValkeyDeduplicator {
	return &ValkeyDeduplicator{client: client}
}

// IsDuplicate checks if the given key already exists via SET NX with TTL.
func (d *ValkeyDeduplicator) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	cmd := d.client.B().Set().Key(key).Value("1").Nx().Ex(ttl).Build()
	resp := d.client.Do(ctx, cmd)

	err := resp.Error()
	if valkey.IsValkeyNil(err) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("dedup set nx %s: %w", key, err)
	}

	return false, nil
}
