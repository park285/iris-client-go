package dedup_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/internal/dedup"
	"github.com/park285/iris-client-go/webhook"
)

const valkeyAddrEnv = "IRIS_CLIENT_VALKEY_TEST_ADDR"

func TestMain(m *testing.M) {
	if os.Getenv(valkeyAddrEnv) == "" {
		fmt.Fprintf(
			os.Stderr,
			"notice: %s is unset; skipping Valkey integration tests - the Lua reserve/commit/release scripts are NOT executed by the remaining tests\n",
			valkeyAddrEnv,
		)
	}

	os.Exit(m.Run())
}

func newIntegrationClient(t *testing.T) valkey.Client {
	t.Helper()

	addr := os.Getenv(valkeyAddrEnv)
	if addr == "" {
		t.Skipf("%s is unset; skipping the Valkey Lua contract integration test", valkeyAddrEnv)
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient(%q) error = %v", addr, err)
	}
	t.Cleanup(client.Close)

	return client
}

func integrationKey(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("iris:msg:{go-test-%s-%d}", t.Name(), time.Now().UnixNano())
}

func valkeyGet(t *testing.T, client valkey.Client, key string) (string, bool) {
	t.Helper()

	value, err := client.Do(t.Context(), client.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("GET %s error = %v", key, err)
	}

	return value, true
}

func valkeyTTL(t *testing.T, client valkey.Client, key string) time.Duration {
	t.Helper()

	millis, err := client.Do(t.Context(), client.B().Pttl().Key(key).Build()).ToInt64()
	if err != nil {
		t.Fatalf("PTTL %s error = %v", key, err)
	}

	return time.Duration(millis) * time.Millisecond
}

func TestIntegrationReserveIsExclusiveAndTokenBound(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	token, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil || state != webhook.DedupStateReserved || token == "" {
		t.Fatalf("Reserve() = %q, %v, %v, want an owner token with DedupStateReserved", token, state, err)
	}

	stored, ok := valkeyGet(t, client, key)
	if !ok || stored != token {
		t.Fatalf("stored value = %q (exists=%v), want the owner token %q", stored, ok, token)
	}
	if ttl := valkeyTTL(t, client, key); ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("PTTL = %v, want a positive TTL no greater than the requested 30s", ttl)
	}

	secondToken, secondState, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil {
		t.Fatalf("second Reserve() error = %v", err)
	}
	if secondState != webhook.DedupStatePending {
		t.Fatalf("second Reserve() state = %v, want DedupStatePending", secondState)
	}
	if secondToken != "" {
		t.Fatalf("second Reserve() token = %q, want empty", secondToken)
	}
	if stored, _ := valkeyGet(t, client, key); stored != token {
		t.Fatalf("stored value = %q, want the first owner's token %q to survive", stored, token)
	}
}

func TestIntegrationCommitVerifiesTokenAndResetsTTL(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	token, _, err := deduplicator.Reserve(t.Context(), key, 2*time.Second)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	if err := deduplicator.Commit(t.Context(), key, "p:foreign", time.Minute); !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("Commit(foreign token) error = %v, want ErrDedupReservationLost", err)
	}
	if stored, _ := valkeyGet(t, client, key); stored != token {
		t.Fatalf("stored value = %q, want the pending owner token %q untouched", stored, token)
	}

	if err := deduplicator.Commit(t.Context(), key, token, time.Minute); err != nil {
		t.Fatalf("Commit(owner token) error = %v", err)
	}

	stored, ok := valkeyGet(t, client, key)
	if !ok || stored != "c" {
		t.Fatalf("stored value = %q (exists=%v), want the committed marker %q", stored, ok, "c")
	}
	// 상한이 없으면 PX가 EX로 바뀌어 TTL이 60000초가 되어도 통과한다.
	if ttl := valkeyTTL(t, client, key); ttl <= 2*time.Second || ttl > time.Minute {
		t.Fatalf("PTTL = %v, want the committed TTL (1m) in milliseconds to replace the short pending TTL", ttl)
	}

	_, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil || state != webhook.DedupStateCommitted {
		t.Fatalf("Reserve() after commit = %v, %v, want DedupStateCommitted", state, err)
	}
}

func TestIntegrationReleaseIsCompareAndDelete(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	token, _, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	err = deduplicator.ReleaseReservation(t.Context(), key, "p:foreign")
	if !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("ReleaseReservation(foreign token) error = %v, want ErrDedupReservationLost", err)
	}
	if stored, ok := valkeyGet(t, client, key); !ok || stored != token {
		t.Fatalf("stored value = %q (exists=%v), want the owner's reservation to survive", stored, ok)
	}

	if err := deduplicator.ReleaseReservation(t.Context(), key, token); err != nil {
		t.Fatalf("ReleaseReservation(owner token) error = %v", err)
	}
	if _, ok := valkeyGet(t, client, key); ok {
		t.Fatal("owner release did not delete the key")
	}

	_, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil || state != webhook.DedupStateReserved {
		t.Fatalf("Reserve() after release = %v, %v, want DedupStateReserved", state, err)
	}
}

// 예약 TTL이 만료된 뒤 다른 요청이 같은 키를 재예약하면, 늦게 도착한 원래 owner의 commit이
// 그 예약을 덮어써서는 안 된다. 이 경계가 무너지면 재예약된 요청이 처리되기 전에 확정으로
// 표시되어 유실된다.
func TestIntegrationLateCommitAfterExpiryDoesNotOverwriteNewOwner(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	staleToken, _, err := deduplicator.Reserve(t.Context(), key, 150*time.Millisecond)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := valkeyGet(t, client, key); !ok {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, ok := valkeyGet(t, client, key); ok {
		t.Fatal("pending reservation did not expire within its TTL")
	}

	newToken, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil || state != webhook.DedupStateReserved {
		t.Fatalf("re-Reserve() = %v, %v, want DedupStateReserved", state, err)
	}

	err = deduplicator.Commit(t.Context(), key, staleToken, time.Minute)
	if !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("late Commit() error = %v, want ErrDedupReservationLost", err)
	}
	if stored, _ := valkeyGet(t, client, key); stored != newToken {
		t.Fatalf("stored value = %q, want the new owner's token %q", stored, newToken)
	}

	err = deduplicator.ReleaseReservation(t.Context(), key, staleToken)
	if !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("late ReleaseReservation() error = %v, want ErrDedupReservationLost", err)
	}
	if stored, ok := valkeyGet(t, client, key); !ok || stored != newToken {
		t.Fatalf("stored value = %q (exists=%v), want the new owner's reservation intact", stored, ok)
	}
}

// 구버전 IsDuplicate가 남긴 "1" 값은 예약이 아니라 확정으로 읽어야 롤링 배포 중 중복이
// 503이 아니라 기존과 같은 200으로 흡수된다. 다만 이 분기는 종단 드레인 경로이므로 별도
// 코드로 계상되어야 "legacy 값이 언제 사라졌는가"를 판정할 수 있다.
func TestIntegrationLegacyValueReadsAsCommitted(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	if duplicate, err := deduplicator.IsDuplicate(t.Context(), key, time.Minute); err != nil || duplicate {
		t.Fatalf("IsDuplicate() = %v, %v, want false, nil on first write", duplicate, err)
	}

	token, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if state != webhook.DedupStateCommitted {
		t.Fatalf("Reserve() state = %v, want DedupStateCommitted for a legacy value", state)
	}
	if token != "" {
		t.Fatalf("Reserve() token = %q, want empty", token)
	}
	if stored, _ := valkeyGet(t, client, key); stored != "1" {
		t.Fatalf("stored value = %q, want the legacy value to be left untouched", stored)
	}
	if got := deduplicator.LegacyCommittedReads(); got != 1 {
		t.Fatalf("LegacyCommittedReads() = %d, want 1; the legacy drain must be observable", got)
	}
}

func TestIntegrationCommittedMarkerIsNotCountedAsLegacy(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	token, _, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := deduplicator.Commit(t.Context(), key, token, time.Minute); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if _, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second); err != nil || state != webhook.DedupStateCommitted {
		t.Fatalf("Reserve() = %v, %v, want DedupStateCommitted", state, err)
	}
	if got := deduplicator.LegacyCommittedReads(); got != 0 {
		t.Fatalf("LegacyCommittedReads() = %d, want 0 for a current committed marker", got)
	}
}

// 알 수 없는 값은 확정으로 접지 말고 오류로 만들어야 한다. 확정으로 읽으면 이 버전이
// 메시지를 200으로 버리고, 오류면 호출자가 fail-open으로 처리를 진행한다.
func TestIntegrationUnknownStoredValueIsAnError(t *testing.T) {
	client := newIntegrationClient(t)
	deduplicator := dedup.NewValkeyDeduplicator(client)
	key := integrationKey(t)
	t.Cleanup(func() {
		client.Do(context.Background(), client.B().Del().Key(key).Build())
	})

	set := client.B().Set().Key(key).Value("x-unknown-marker").Ex(time.Minute).Build()
	if err := client.Do(t.Context(), set).Error(); err != nil {
		t.Fatalf("SET %s error = %v", key, err)
	}

	token, state, err := deduplicator.Reserve(t.Context(), key, 30*time.Second)
	if err == nil {
		t.Fatalf("Reserve() = %q, %v, nil, want an error so the caller fails open", token, state)
	}
	if token != "" {
		t.Fatalf("Reserve() token = %q, want empty; no reservation was written", token)
	}
	if stored, _ := valkeyGet(t, client, key); stored != "x-unknown-marker" {
		t.Fatalf("stored value = %q, want the unknown value left untouched", stored)
	}
}
