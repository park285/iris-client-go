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

	var _ webhook.Deduplicator = (*dedup.ValkeyDeduplicator)(nil)
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

	wantCommands := []string{"SET", "message:1", "1", "NX", "EX", "300"}
	if !slices.Equal(client.commands, wantCommands) {
		t.Fatalf("commands = %v, want %v", client.commands, wantCommands)
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
