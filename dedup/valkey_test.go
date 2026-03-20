package dedup_test

import (
	"testing"

	"park285/iris-client-go/dedup"
	"park285/iris-client-go/webhook"
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
