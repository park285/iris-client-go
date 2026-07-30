package dedup

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
)

func TestIntegrationReserveScriptIsSelfIdempotent(t *testing.T) {
	addr := os.Getenv("IRIS_CLIENT_VALKEY_TEST_ADDR")
	if addr == "" {
		t.Skip("IRIS_CLIENT_VALKEY_TEST_ADDR is unset; skipping the Valkey Lua contract integration test")
	}

	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey.NewClient(%q) error = %v", addr, err)
	}
	t.Cleanup(client.Close)

	key := fmt.Sprintf("iris:msg:{go-test-%s-%d}", t.Name(), time.Now().UnixNano())
	t.Cleanup(func() {
		client.Do(t.Context(), client.B().Del().Key(key).Build())
	})

	token := pendingValuePrefix + "self-idempotent"
	args := []string{token, "30000"}

	first, err := reserveScript.Exec(t.Context(), client, []string{key}, args).ToInt64()
	if err != nil || first != codeReserved {
		t.Fatalf("first reserve = %d, %v, want %d", first, err, codeReserved)
	}

	second, err := reserveScript.Exec(t.Context(), client, []string{key}, args).ToInt64()
	if err != nil {
		t.Fatalf("retransmitted reserve error = %v", err)
	}
	if second != codeReserved {
		t.Fatalf("retransmitted reserve = %d, want %d; the owner must not be blocked by its own reservation", second, codeReserved)
	}
}
