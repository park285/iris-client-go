package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/webhook"
)

// ValkeyDeduplicator는 Valkey SET NX를 사용하여 webhook.Deduplicator를 구현합니다.
type ValkeyDeduplicator struct {
	client valkey.Client
}

var _ webhook.Deduplicator = (*ValkeyDeduplicator)(nil)

func NewValkeyDeduplicator(client valkey.Client) *ValkeyDeduplicator {
	return &ValkeyDeduplicator{client: client}
}

// IsDuplicate는 SET NX와 TTL을 사용하여 주어진 키의 존재 여부를 확인합니다.
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
