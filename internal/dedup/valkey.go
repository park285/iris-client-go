package dedup

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/v2/internal/client/randomhex"
	"github.com/park285/iris-client-go/v2/webhook"
)

const (
	pendingValuePrefix = "p:"
	committedValue     = "c"

	codeReserved  = 1
	codePending   = 2
	codeCommitted = 3
	codeOK        = 1
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
return -1
`

var reserveScript = valkey.NewLuaScript(reserveScriptBody)

// current == false는 예약이 저장소에 닿지 못했거나 pending TTL이 먼저 만료된 경우다. 아무도
// 소유하지 않는 키이므로 확정으로 덮는 것이 안전하고, 덮지 않으면 Iris 재전송 지평 내내 같은
// 메시지가 다시 처리된다.
//
// 확정 마커는 owner를 담지 않으므로 current == ARGV[2]를 성공으로 보면 "내가 쓴 마커"와
// "내 예약을 가로챈 다른 소비자가 쓴 마커"가 구분되지 않는다. 후자가 곧 이중 전달이고
// ErrDedupReservationLost가 그 유일한 신호라, 여기서는 다른 owner의 확정도 0으로 남긴다.
var commitScript = valkey.NewLuaScript(`
local current = redis.call('GET', KEYS[1])
if current == false or current == ARGV[1] then
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

type ValkeyMessageDeduplicator struct {
	client valkey.Client
}

type ValkeyNonceStore struct {
	client valkey.Client
}

var _ webhook.MessageDeduplicator = (*ValkeyMessageDeduplicator)(nil)
var _ webhook.SetOnceNonceStore = (*ValkeyNonceStore)(nil)

func NewValkeyMessageDeduplicator(client valkey.Client) *ValkeyMessageDeduplicator {
	return &ValkeyMessageDeduplicator{client: client}
}

func NewValkeyNonceStore(client valkey.Client) *ValkeyNonceStore {
	return &ValkeyNonceStore{client: client}
}

// IsDuplicate는 SET NX와 TTL을 사용하여 주어진 키의 존재 여부를 확인합니다.
func (s *ValkeyNonceStore) IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error) {
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

// Reserve는 키가 비어 있을 때만 owner token을 심고 그 상태를 반환합니다.
func (d *ValkeyMessageDeduplicator) Reserve(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, webhook.DedupState, error) {
	// 명령을 보내기 전에 이미 죽은 ctx면 예약이 저장소에 닿을 수 없다. 반대로 전송 후
	// 만료된 ctx는 서버가 SET을 끝냈을 수 있으므로 token을 넘겨 회수하게 둔다.
	if err := ctx.Err(); err != nil {
		return "", webhook.DedupStateReserved, fmt.Errorf("dedup reserve: %w", err)
	}

	token := pendingValuePrefix + randomhex.Generate()

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

// SetOnceNonce는 IsDuplicate가 SET NX 단일 왕복이라 set-once fail-closed임을 선언합니다.
func (s *ValkeyNonceStore) SetOnceNonce() {}

// Commit은 token이 예약을 소유한 경우에만 키를 확정 상태로 바꿉니다.
func (d *ValkeyMessageDeduplicator) Commit(ctx context.Context, key, token string, ttl time.Duration) error {
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
func (d *ValkeyMessageDeduplicator) ReleaseReservation(ctx context.Context, key, token string) error {
	code, err := releaseScript.Exec(ctx, d.client, []string{key}, []string{token}).ToInt64()
	if err != nil {
		return fmt.Errorf("dedup release: %w", err)
	}
	if code != codeOK {
		return fmt.Errorf("dedup release: %w", webhook.ErrDedupReservationLost)
	}

	return nil
}

// Ex는 TTL을 초로 절사하므로 1초 미만 TTL이 EX 0이 되고, Valkey가 그 명령 자체를 거부해
// 모든 요청이 저장소 오류(503)로 떨어진다. Reserve/Commit과 같은 밀리초 단위에 1ms floor를
// 두어 오설정을 짧은 TTL로 흡수한다.
func flooredTTL(ttl time.Duration) time.Duration {
	return max(ttl, time.Millisecond)
}

func millisArg(ttl time.Duration) string {
	return strconv.FormatInt(flooredTTL(ttl).Milliseconds(), 10)
}
