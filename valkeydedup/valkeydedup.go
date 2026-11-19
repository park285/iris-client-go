package valkeydedup

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/v5/webhook"
)

type NonceStore struct {
	client valkey.Client
}

var _ webhook.SetOnceNonceStore = (*NonceStore)(nil)

func NewNonceStore(valkeyClient valkey.Client) *NonceStore {
	return &NonceStore{client: valkeyClient}
}

func (s *NonceStore) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	cmd := s.client.B().Set().Key(key).Value("1").Nx().Px(flooredTTL(ttl)).Build()
	resp := s.client.Do(ctx, cmd)

	err := resp.Error()
	if valkey.IsValkeyNil(err) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("dedup set nx %s: %w", key, err)
	}

	return false, nil
}

func (s *NonceStore) SetOnceNonce() {}

func flooredTTL(ttl time.Duration) time.Duration {
	return max(ttl, time.Millisecond)
}
