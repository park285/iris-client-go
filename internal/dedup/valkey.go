package dedup

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/internal/client/randomhex"
	"github.com/park285/iris-client-go/webhook"
)

const (
	pendingValuePrefix = "p:"
	committedValue     = "c"

	codeReserved        = 1
	codePending         = 2
	codeCommitted       = 3
	codeLegacyCommitted = 4
	codeOK              = 1
)

// 알려진 값만 상태로 매핑하고 나머지는 음수로 떨어뜨린다. catch-all로 확정 처리하면 키
// 공간 오염이나 미래 마커를 이 버전이 "이미 처리됨"으로 읽어 메시지를 200으로 버린다.
//
// 첫 분기는 self-idempotency다. NewLuaScript(비-retryable)를 쓰므로 정상 경로에서는 같은
// EVALSHA가 재전송되지 않지만, 이 가드가 없으면 재전송 시 자기 예약을 pending으로 읽어
// 자기 자신 때문에 503을 내보낸다.
const reserveScriptBody = `
local current = redis.call('GET', KEYS[1])
if current == ARGV[1] then
  return 1
end
if current == false then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if string.sub(current, 1, 2) == 'p:' then
  return 2
end
if current == 'c' then
  return 3
end
if current == '1' then
  return 4
end
return -1
`

var reserveScript = valkey.NewLuaScript(reserveScriptBody)

var commitScript = valkey.NewLuaScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  return 1
end
return 0
`)

var releaseScript = valkey.NewLuaScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

type ValkeyDeduplicator struct {
	client valkey.Client

	legacyCommittedReads atomic.Uint64
}

var (
	_ webhook.StatefulDeduplicator = (*ValkeyDeduplicator)(nil)
	_ webhook.SetOnceNonceStore    = (*ValkeyDeduplicator)(nil)
)

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

// Reserve는 키가 비어 있을 때만 owner token을 심고 그 상태를 반환합니다.
func (d *ValkeyDeduplicator) Reserve(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, webhook.DedupState, error) {
	// 명령을 보내기 전에 이미 죽은 ctx면 예약이 저장소에 닿을 수 없다. 반대로 전송 후
	// 만료된 ctx는 서버가 SET을 끝냈을 수 있으므로 token을 넘겨 회수하게 둔다.
	if err := ctx.Err(); err != nil {
		return "", webhook.DedupStateReserved, fmt.Errorf("dedup reserve: %w", err)
	}

	token := pendingValuePrefix + randomhex.Generate("iris-dedup")

	code, err := reserveScript.Exec(ctx, d.client, []string{key}, []string{token, millisArg(ttl)}).ToInt64()
	if err != nil {
		return reserveErrorToken(token, err), webhook.DedupStateReserved, fmt.Errorf("dedup reserve: %w", err)
	}

	switch code {
	case codeReserved:
		return token, webhook.DedupStateReserved, nil
	case codePending:
		return "", webhook.DedupStatePending, nil
	case codeCommitted:
		return "", webhook.DedupStateCommitted, nil
	case codeLegacyCommitted:
		d.legacyCommittedReads.Add(1)

		return "", webhook.DedupStateCommitted, nil
	default:
		return "", webhook.DedupStateReserved, fmt.Errorf(
			"dedup reserve: unknown stored value for %s (state code %d)", key, code,
		)
	}
}

// reserveScript는 GET 뒤의 SET이 유일한 쓰기이자 마지막 단계라, 서버가 낸 오류는 SET에 닿기
// 전에 끝났다는 증명이 된다. 전송 실패나 응답 유실은 그 구분이 불가능하므로 token을 넘겨
// 호출자가 회수하게 한다.
func reserveErrorToken(token string, err error) string {
	if _, serverResponded := valkey.IsValkeyErr(err); serverResponded {
		return ""
	}

	return token
}

// LegacyCommittedReads는 구버전 IsDuplicate가 남긴 "1" 값을 확정으로 읽은 횟수입니다.
// 이 값이 배포 후 계속 0이면 legacy 값이 모두 만료된 것이므로, 그 종단 분기를 제거할 수
// 있는지 판단하는 근거가 됩니다.
func (d *ValkeyDeduplicator) LegacyCommittedReads() uint64 {
	return d.legacyCommittedReads.Load()
}

// SetOnceNonce는 IsDuplicate가 SET NX 단일 왕복이라 set-once fail-closed임을 선언합니다.
func (d *ValkeyDeduplicator) SetOnceNonce() {}

// Commit은 token이 예약을 소유한 경우에만 키를 확정 상태로 바꿉니다.
func (d *ValkeyDeduplicator) Commit(ctx context.Context, key, token string, ttl time.Duration) error {
	code, err := commitScript.Exec(
		ctx,
		d.client,
		[]string{key},
		[]string{token, committedValue, millisArg(ttl)},
	).ToInt64()
	if err != nil {
		return fmt.Errorf("dedup commit: %w", err)
	}
	if code != codeOK {
		return fmt.Errorf("dedup commit: %w", webhook.ErrDedupReservationLost)
	}

	return nil
}

// ReleaseReservation은 token이 소유한 예약만 삭제하고 다른 owner의 키는 건드리지 않습니다.
func (d *ValkeyDeduplicator) ReleaseReservation(ctx context.Context, key, token string) error {
	code, err := releaseScript.Exec(ctx, d.client, []string{key}, []string{token}).ToInt64()
	if err != nil {
		return fmt.Errorf("dedup release: %w", err)
	}
	if code != codeOK {
		return fmt.Errorf("dedup release: %w", webhook.ErrDedupReservationLost)
	}

	return nil
}

// Release는 소유권을 검증하지 않고 키를 삭제합니다.
//
// Deprecated: 같은 키를 재예약한 다른 요청의 예약까지 지울 수 있습니다.
// ReleaseReservation을 사용하십시오. Handler는 이 경로를 호출하지 않습니다.
func (d *ValkeyDeduplicator) Release(ctx context.Context, key string) error {
	cmd := d.client.B().Del().Key(key).Build()
	if err := d.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("dedup release: %w", err)
	}

	return nil
}

func millisArg(ttl time.Duration) string {
	return strconv.FormatInt(max(ttl.Milliseconds(), 1), 10)
}
