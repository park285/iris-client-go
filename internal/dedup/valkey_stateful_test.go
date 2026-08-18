package dedup_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/v2/internal/dedup"
	"github.com/park285/iris-client-go/v2/webhook"
)

const (
	valkeyTypeInteger = byte(':')
	valkeyTypeError   = byte('-')
)

func valkeyServerError(message string) *valkey.ValkeyError {
	layout := valkeyMessageLayout{
		bytes:  unsafe.StringData(message),
		intlen: int64(len(message)),
		typ:    valkeyTypeError,
	}

	return (*valkey.ValkeyError)(unsafe.Pointer(&layout))
}

type valkeyMessageLayout struct {
	attrs  *valkeyMessageLayout
	bytes  *byte
	array  *valkeyMessageLayout
	intlen int64
	typ    byte
	ttl    [7]byte
}

func valkeyIntegerResult(value int64) valkey.ValkeyResult {
	message := valkeyMessageLayout{intlen: value, typ: valkeyTypeInteger}

	return *(*valkey.ValkeyResult)(unsafe.Pointer(&valkeyResultLayout{
		val: *(*valkey.ValkeyMessage)(unsafe.Pointer(&message)),
	}))
}

func TestValkeyMessageLayoutMirrorsUpstream(t *testing.T) {
	t.Parallel()

	if unsafe.Sizeof(valkey.ValkeyMessage{}) != unsafe.Sizeof(valkeyMessageLayout{}) {
		t.Fatalf(
			"valkey.ValkeyMessage size = %d, mirror size = %d; update the test mirror",
			unsafe.Sizeof(valkey.ValkeyMessage{}),
			unsafe.Sizeof(valkeyMessageLayout{}),
		)
	}
	if unsafe.Sizeof(valkey.ValkeyResult{}) != unsafe.Sizeof(valkeyResultLayout{}) {
		t.Fatalf(
			"valkey.ValkeyResult size = %d, mirror size = %d; update the test mirror",
			unsafe.Sizeof(valkey.ValkeyResult{}),
			unsafe.Sizeof(valkeyResultLayout{}),
		)
	}
	if unsafe.Sizeof(valkey.Builder{}) != unsafe.Sizeof(valkeyBuilderLayout{}) {
		t.Fatalf(
			"valkey.Builder size = %d, mirror size = %d; update the test mirror",
			unsafe.Sizeof(valkey.Builder{}),
			unsafe.Sizeof(valkeyBuilderLayout{}),
		)
	}

	value, err := valkeyIntegerResult(7).ToInt64()
	if err != nil || value != 7 {
		t.Fatalf("valkeyIntegerResult(7).ToInt64() = %d, %v, want 7, nil", value, err)
	}
}

func TestValkeyMessageDeduplicatorImplementsStatefulContract(t *testing.T) {
	t.Parallel()

	var _ webhook.MessageDeduplicator = (*dedup.ValkeyMessageDeduplicator)(nil)
}

func TestValkeyMessageDeduplicatorReserveTakesOwnership(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(1)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	token, state, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", time.Minute)
	if err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if state != webhook.DedupStateReserved {
		t.Fatalf("Reserve() state = %v, want DedupStateReserved", state)
	}
	if !strings.HasPrefix(token, "p:") || len(token) <= len("p:") {
		t.Fatalf("Reserve() token = %q, want a non-empty pending-prefixed owner token", token)
	}

	commands := client.commands
	if len(commands) != 6 {
		t.Fatalf("commands = %v, want EVALSHA sha numkeys key token ttl", commands)
	}
	if commands[0] != "EVALSHA" || commands[2] != "1" || commands[3] != "iris:msg:{m1}" {
		t.Fatalf("commands = %v, want a single-key EVALSHA on the dedup key", commands)
	}
	if commands[4] != token {
		t.Fatalf("commands[4] = %q, want the owner token %q", commands[4], token)
	}
	if commands[5] != "60000" {
		t.Fatalf("commands[5] = %q, want the reservation TTL in milliseconds", commands[5])
	}
}

func TestValkeyMessageDeduplicatorReserveStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code int64
		want webhook.DedupState
	}{
		{name: "pending", code: 2, want: webhook.DedupStatePending},
		{name: "committed", code: 3, want: webhook.DedupStateCommitted},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := &mockValkeyClient{result: valkeyIntegerResult(testCase.code)}
			deduplicator := dedup.NewValkeyMessageDeduplicator(client)

			token, state, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", time.Minute)
			if err != nil {
				t.Fatalf("Reserve() error = %v, want nil", err)
			}
			if state != testCase.want {
				t.Fatalf("Reserve() state = %v, want %v", state, testCase.want)
			}
			if token != "" {
				t.Fatalf("Reserve() token = %q, want empty when ownership was not taken", token)
			}
		})
	}
}

func TestValkeyMessageDeduplicatorReserveUnexpectedCode(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(0)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	if _, _, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", time.Minute); err == nil {
		t.Fatal("Reserve() error = nil, want an unexpected state code error")
	}
}

func TestValkeyMessageDeduplicatorReserveValkeyError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	client := &mockValkeyClient{result: valkeyResultWithError(boom)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	token, state, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("Reserve() error = %v, want wrapping %v", err, boom)
	}
	if state != webhook.DedupStateReserved {
		t.Fatalf("Reserve() state = %v, want DedupStateReserved", state)
	}
	if !strings.HasPrefix(token, "p:") || len(token) <= len("p:") {
		t.Fatalf("Reserve() token = %q, want the attempted owner token so the caller can reclaim a possible orphan", token)
	}
}

func TestValkeyMessageDeduplicatorReserveDropsTokenOnServerError(t *testing.T) {
	t.Parallel()

	serverErr := valkeyServerError("WRONGTYPE Operation against a key holding the wrong kind of value")
	client := &mockValkeyClient{result: valkeyResultWithError(serverErr)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	token, _, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", time.Minute)
	if !errors.Is(err, serverErr) {
		t.Fatalf("Reserve() error = %v, want wrapping %v", err, serverErr)
	}
	if token != "" {
		t.Fatalf("Reserve() token = %q, want empty when the server answered and no reservation was written", token)
	}
}

func TestValkeyMessageDeduplicatorReserveDropsTokenForDeadContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := &mockValkeyClient{result: valkeyIntegerResult(1)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	token, _, err := deduplicator.Reserve(ctx, "iris:msg:{m1}", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Reserve() error = %v, want wrapping context.Canceled", err)
	}
	if token != "" {
		t.Fatalf("Reserve() token = %q, want empty when the command was never sent", token)
	}
	if len(client.commands) != 0 {
		t.Fatalf("commands = %v, want none for an already-cancelled context", client.commands)
	}
}

func TestValkeyMessageDeduplicatorReserveSubSecondTTL(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(1)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	if _, _, err := deduplicator.Reserve(t.Context(), "iris:msg:{m1}", 100*time.Microsecond); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if commands := client.commands; commands[len(commands)-1] != "1" {
		t.Fatalf(
			"commands = %v, want a clamped 1ms TTL argument; a sub-millisecond TTL truncates to 0 and the server rejects "+
				"SET ... PX 0 with \"ERR invalid expire time\", which would surface as a reserve failure instead of a reservation",
			commands,
		)
	}
}

func TestValkeyMessageDeduplicatorCommitMarksCommitted(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(1)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	if err := deduplicator.Commit(t.Context(), "iris:msg:{m1}", "p:owner", time.Minute); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	commands := client.commands
	if commands[0] != "EVALSHA" {
		t.Fatalf("commands[0] = %q, want EVALSHA", commands[0])
	}
	if !slices.Contains(commands, "p:owner") || !slices.Contains(commands, "c") {
		t.Fatalf("commands = %v, want the owner token and the committed marker", commands)
	}
}

func TestValkeyMessageDeduplicatorCommitForeignTokenReportsLostReservation(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(0)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	err := deduplicator.Commit(t.Context(), "iris:msg:{m1}", "p:foreign", time.Minute)
	if !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("Commit() error = %v, want ErrDedupReservationLost", err)
	}
}

func TestValkeyMessageDeduplicatorReleaseReservationDeletesOwnedKeyOnly(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(1)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	if err := deduplicator.ReleaseReservation(t.Context(), "iris:msg:{m1}", "p:owner"); err != nil {
		t.Fatalf("ReleaseReservation() error = %v, want nil", err)
	}

	commands := client.commands
	if commands[0] != "EVALSHA" {
		t.Fatalf("commands[0] = %q, want EVALSHA", commands[0])
	}
	if slices.Contains(commands, "DEL") {
		t.Fatalf("commands = %v, want no unconditional DEL", commands)
	}
	if !slices.Contains(commands, "p:owner") {
		t.Fatalf("commands = %v, want the owner token as a script ARGV", commands)
	}
}

func TestValkeyMessageDeduplicatorReleaseReservationForeignTokenIsRejected(t *testing.T) {
	t.Parallel()

	client := &mockValkeyClient{result: valkeyIntegerResult(0)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	err := deduplicator.ReleaseReservation(t.Context(), "iris:msg:{m1}", "p:foreign")
	if !errors.Is(err, webhook.ErrDedupReservationLost) {
		t.Fatalf("ReleaseReservation() error = %v, want ErrDedupReservationLost", err)
	}
	if commands := client.commands; slices.Contains(commands, "DEL") {
		t.Fatalf("commands = %v, want no DEL for a foreign token", commands)
	}
}

func TestValkeyMessageDeduplicatorReleaseReservationValkeyError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	client := &mockValkeyClient{result: valkeyResultWithError(boom)}
	deduplicator := dedup.NewValkeyMessageDeduplicator(client)

	if err := deduplicator.ReleaseReservation(t.Context(), "iris:msg:{m1}", "p:owner"); !errors.Is(err, boom) {
		t.Fatalf("ReleaseReservation() error = %v, want wrapping %v", err, boom)
	}
}
