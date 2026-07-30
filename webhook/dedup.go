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

// SetOnceNonceStore는 IsDuplicate가 set-once fail-closed임을 backend가 선언하는 선택적
// 마커입니다. 구현하면 Handler는 이 backend로의 암묵적 nonce cache fallback을 warn하지
// 않습니다. IsDuplicate가 키를 원자적으로 한 번만 기록하고, 기록에 실패하면 오류를
// 반환하는 backend만 이 마커를 구현해야 합니다.
type SetOnceNonceStore interface {
	NonceStore

	SetOnceNonce()
}

// Deduplicator는 non-durable 모드의 message dedup backend 계약입니다.
//
// IsDuplicate는 예약(reserve) 시맨틱입니다. 최초 관측이면 키를 기록합니다. 예약이
// admission보다 먼저 확정되므로 enqueue 실패 후의 정상 재전송이 중복으로 흡수되어
// 유실됩니다.
//
// Deprecated: StatefulDeduplicator로 이전하십시오. 이 계약만 구현한 backend를
// WithDeduplicator로 주입하면 Handler가 기동 시 warn하고 위의 유실 경로가 그대로 남습니다.
// nonce 저장소를 구현할 때는 이 타입이 아니라 NonceStore를 사용하십시오 — 그 역할은
// 사용 중단 대상이 아닙니다.
type Deduplicator interface {
	NonceStore
}

// DedupReleaser는 legacy Deduplicator가 예약을 되돌리는 선택적 계약입니다.
//
// Deprecated: 소유권을 검증하지 않으므로 같은 키를 재예약한 다른 요청의 예약까지 지울 수
// 있습니다. 새 backend는 StatefulDeduplicator를 구현하십시오. Handler는 backend가
// StatefulDeduplicator를 구현하지 않을 때만 이 경로를 사용합니다.
type DedupReleaser interface {
	Release(ctx context.Context, key string) error
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

// StatefulDeduplicator는 예약과 admission 확정을 분리하는 token-bound dedup 계약입니다.
// WithDeduplicator로 주입한 backend가 이 인터페이스를 구현하면 Handler는 legacy 경로 대신
// 다음 순서를 사용합니다.
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
// token으로 fail-open 처리 후 Commit 또는 ReleaseReservation을 시도해 고아 예약을 회수합니다.
// 명령을 보내기 전에 만료된 context는 그 증명입니다. 저장소가 낸 오류는 예약 쓰기가 단일
// 왕복의 마지막 변경 단계인 backend에서만 증명이 됩니다 — 쓰기 뒤에 다른 왕복이나 변경이
// 남아 있으면 그 오류는 이미 저장된 예약을 부정하지 못합니다.
//
// NonceStore를 함께 구현해야 합니다. 명시적인 WithNonceCache가 없으면 Handler가 같은 값을
// HMAC nonce cache로도 사용하기 때문입니다.
type StatefulDeduplicator interface {
	NonceStore

	Reserve(ctx context.Context, key string, ttl time.Duration) (string, DedupState, error)
	Commit(ctx context.Context, key, token string, ttl time.Duration) error
	ReleaseReservation(ctx context.Context, key, token string) error
}

type NoopDeduplicator struct{}

func (NoopDeduplicator) IsDuplicate(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, nil
}
