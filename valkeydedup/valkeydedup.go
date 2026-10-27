package valkeydedup

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/v4/webhook"
)

// NonceStore는 Valkey SET NX 한 번으로 webhook HMAC nonce를 기록하는 set-once 저장소입니다.
type NonceStore struct {
	client valkey.Client
}

var _ webhook.SetOnceNonceStore = (*NonceStore)(nil)

func NewNonceStore(valkeyClient valkey.Client) *NonceStore {
	return &NonceStore{client: valkeyClient}
}

// IsDuplicate는 SET NX와 TTL을 사용하여 주어진 키의 존재 여부를 확인합니다.
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

// SetOnceNonce는 IsDuplicate가 SET NX 단일 왕복이라 set-once fail-closed임을 선언합니다.
func (s *NonceStore) SetOnceNonce() {}

// PX는 밀리초 단위라 1ms 미만 TTL이 PX 0이 되고, Valkey가 그 명령 자체를 거부해 모든 요청이
// 저장소 오류(503)로 떨어진다. 1ms floor를 두어 오설정을 짧은 TTL로 흡수한다.
func flooredTTL(ttl time.Duration) time.Duration {
	return max(ttl, time.Millisecond)
}
