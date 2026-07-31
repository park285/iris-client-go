package valkeydedup

import (
	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/internal/dedup"
	"github.com/park285/iris-client-go/webhook"
)

type Deduplicator = dedup.ValkeyDeduplicator

func New(valkeyClient valkey.Client) *Deduplicator {
	return dedup.NewValkeyDeduplicator(valkeyClient)
}

func Option(valkeyClient valkey.Client) webhook.HandlerOption {
	deduplicator := New(valkeyClient)
	return func(handler *webhook.Handler) {
		webhook.WithDeduplicator(deduplicator)(handler)
		webhook.WithNonceCache(deduplicator)(handler)
	}
}
