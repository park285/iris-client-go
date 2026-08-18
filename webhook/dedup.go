package webhook

import (
	"context"
	"errors"
	"time"
)

// NonceStore는 HMAC replay 방지용 nonce 저장소입니다. IsDuplicate는 최초 관측이면 키를
// 기록하고, 기록에 실패하면 오류를 반환해야 합니다(fail-closed). 이 역할은 message dedup의
// 상태 계약과 무관하게 유지됩니다.
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

type DedupState int

const (
	DedupStateReserved DedupState = iota
	// 다른 요청이 예약을 쥔 채 아직 commit하지 않은 상태다.
	DedupStatePending
	DedupStateCommitted
)

// ErrDedupReservationLost는 token이 더 이상 해당 키의 예약을 소유하지 않을 때
// Commit/ReleaseReservation이 반환합니다. 예약 TTL 만료 후 다른 요청이 같은 키를
// 재예약했거나 예약이 이미 정리된 경우입니다.
var ErrDedupReservationLost = errors.New("webhook: dedup reservation lost")

// MessageDeduplicator는 예약과 admission 확정을 분리하는 token-bound message dedup 계약입니다.
// WithMessageDeduplicator로 주입하면 Handler는 다음 순서를 사용합니다.
//
//   - Reserve가 DedupStateReserved를 반환하면 요청을 계속 처리합니다.
//   - enqueue가 성공하면 Commit으로 admission을 확정하고 HTTP 200을 반환합니다.
//   - enqueue가 실패하면 ReleaseReservation으로 자신의 token이 쥔 예약만 해제하고
//     HTTP 503을 반환하므로 재전송이 다시 admission될 수 있습니다.
//   - Reserve가 DedupStatePending을 반환하면(선행 요청이 아직 확정 전) HTTP 503을 반환해
//     재전송이 나중에 처리되게 합니다.
//   - Reserve가 DedupStateCommitted를 반환하면 HTTP 200으로 흡수합니다.
//
// Reserve가 반환하는 token은 Handler에게 불투명한 값이며 Commit/ReleaseReservation에
// 그대로 전달됩니다. 다른 token이 쥔 예약은 절대 삭제하거나 덮어써서는 안 되고,
// 그 경우 ErrDedupReservationLost를 반환해야 합니다.
//
// Reserve가 오류를 반환할 때는, 예약이 저장소에 존재할 수 없음을 증명할 수 있을 때만 빈
// token을 반환하십시오. 그 외에는 시도에 쓴 token을 오류와 함께 반환해야 하며, Handler는 그
// token으로 ReleaseReservation을 시도한 뒤 요청을 503으로 거절합니다.
// 명령을 보내기 전에 만료된 context는 그 증명입니다. 저장소가 낸 오류는 예약 쓰기가 단일
// 왕복의 마지막 변경 단계인 backend에서만 증명이 됩니다 — 쓰기 뒤에 다른 왕복이나 변경이
// 남아 있으면 그 오류는 이미 저장된 예약을 부정하지 못합니다.
type MessageDeduplicator interface {
	Reserve(ctx context.Context, key string, ttl time.Duration) (string, DedupState, error)
	Commit(ctx context.Context, key, token string, ttl time.Duration) error
	ReleaseReservation(ctx context.Context, key, token string) error
}
