package dedup_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/park285/iris-client-go/internal/dedup"
	"github.com/park285/iris-client-go/webhook"
	"github.com/valkey-io/valkey-go"
)

func TestValkeyDeduplicatorImplementsInterface(t *testing.T) {
	t.Parallel()

	//lint:ignore SA1019 소비자 drain 전 legacy interface 호환을 검증한다.
	var _ webhook.Deduplicator = (*dedup.ValkeyDeduplicator)(nil)
	//lint:ignore SA1019 소비자 drain 전 legacy interface 호환을 검증한다.
	var _ webhook.DedupReleaser = (*dedup.ValkeyDeduplicator)(nil)
	var _ webhook.StatefulDeduplicator = (*dedup.ValkeyDeduplicator)(nil)
}

func TestValkeyDeduplicatorReleaseDeletesKey(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	if err := deduplicator.Release(t.Context(), "message:1"); err != nil {
		t.Fatalf("Release() error = %v, want nil", err)
	}

	wantCommands := []string{"DEL", "message:1"}
	if !slices.Equal(client.commands, wantCommands) {
		t.Fatalf("commands = %v, want %v", client.commands, wantCommands)
	}
}

func TestValkeyDeduplicatorReleaseValkeyError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	client := &mockValkeyClient{result: valkeyResultWithError(boom)}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	if err := deduplicator.Release(t.Context(), "message:1"); !errors.Is(err, boom) {
		t.Fatalf("Release() error = %v, want wrapping %v", err, boom)
	}
}

func TestNewValkeyDeduplicator(t *testing.T) {
	t.Parallel()

	deduplicator := dedup.NewValkeyDeduplicator(nil)
	if deduplicator == nil {
		t.Fatal("NewValkeyDeduplicator() returned nil")
	}
}

func TestValkeyDeduplicatorIsDuplicateFirstSeen(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	duplicate, err := deduplicator.IsDuplicate(t.Context(), "message:1", 5*time.Minute)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v, want nil", err)
	}
	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}

	wantCommands := []string{"SET", "message:1", "1", "NX", "PX", "300000"}
	if !slices.Equal(client.commands, wantCommands) {
		t.Fatalf("commands = %v, want %v", client.commands, wantCommands)
	}
}

// 초 절사를 쓰면 1초 미만 TTL이 EX 0이 되어 Valkey가 명령을 거부하고, 그 오류가 모든
// 요청을 503으로 되돌린다. Reserve/Commit과 같은 1ms floor로 흡수해야 한다.
func TestValkeyDeduplicatorIsDuplicateFloorsSubMillisecondTTL(t *testing.T) {
	t.Parallel()

	for ttl, wantMillis := range map[time.Duration]string{
		500 * time.Microsecond:  "1",
		0:                       "1",
		-time.Second:            "1",
		1500 * time.Millisecond: "1500",
	} {
		client := &mockValkeyClient{}
		deduplicator := dedup.NewValkeyDeduplicator(client)

		if _, err := deduplicator.IsDuplicate(t.Context(), "message:1", ttl); err != nil {
			t.Fatalf("IsDuplicate(ttl=%v) error = %v", ttl, err)
		}

		wantCommands := []string{"SET", "message:1", "1", "NX", "PX", wantMillis}
		if !slices.Equal(client.commands, wantCommands) {
			t.Fatalf("commands for ttl=%v = %v, want %v", ttl, client.commands, wantCommands)
		}
	}
}

func TestValkeyDeduplicatorIsDuplicateDuplicateKey(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyResultWithError(valkey.Nil)}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	duplicate, err := deduplicator.IsDuplicate(t.Context(), "message:1", time.Minute)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v, want nil", err)
	}
	if !duplicate {
		t.Fatal("IsDuplicate() duplicate = false, want true")
	}
}

func TestValkeyDeduplicatorIsDuplicateValkeyError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	client := &mockValkeyClient{result: valkeyResultWithError(boom)}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	duplicate, err := deduplicator.IsDuplicate(t.Context(), "message:1", time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("IsDuplicate() error = %v, want wrapping %v", err, boom)
	}
	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}
}

func TestValkeyDeduplicatorIsDuplicateContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := &mockValkeyClient{
		do: func(ctx context.Context, _ valkey.Completed) valkey.ValkeyResult {
			return valkeyResultWithError(ctx.Err())
		},
	}
	deduplicator := dedup.NewValkeyDeduplicator(client)

	duplicate, err := deduplicator.IsDuplicate(ctx, "message:1", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IsDuplicate() error = %v, want wrapping %v", err, context.Canceled)
	}
	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}
}

type mockValkeyClient struct {
	valkey.Client

	result   valkey.ValkeyResult
	do       func(context.Context, valkey.Completed) valkey.ValkeyResult
	commands []string
}

func (c *mockValkeyClient) B() valkey.Builder {
	return valkeyBuilder()
}

func (c *mockValkeyClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	c.commands = slices.Clone(cmd.Commands())
	if c.do != nil {
		return c.do(ctx, cmd)
	}

	return c.result
}

type valkeyResultLayout struct {
	err error
	val valkey.ValkeyMessage
}

type valkeyBuilderLayout struct {
	ks uint16
}

const valkeyNoSlot = uint16(1 << 15)

func valkeyResultWithError(err error) valkey.ValkeyResult {
	return *(*valkey.ValkeyResult)(unsafe.Pointer(&valkeyResultLayout{err: err}))
}

func valkeyBuilder() valkey.Builder {
	return *(*valkey.Builder)(unsafe.Pointer(&valkeyBuilderLayout{ks: valkeyNoSlot}))
}
