package webhook

import (
	"context"
	"time"
)

// NonceStore는 HMAC replay 방지용 nonce 저장소입니다. IsDuplicate는 최초 관측이면 키를
// 기록하고, 기록에 실패하면 오류를 반환해야 합니다(fail-closed).
type NonceStore interface {
	IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// SetOnceNonceStore는 IsDuplicate가 set-once fail-closed임을 backend가 선언하는 마커입니다.
// IsDuplicate가 키를 원자적으로 한 번만 기록하고, 기록에 실패하면 오류를 반환하는 backend만
// 이 마커를 구현해야 합니다. Handler는 이 마커가 없는 store를 거절합니다.
type SetOnceNonceStore interface {
	NonceStore

	SetOnceNonce()
}
