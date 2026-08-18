package valkeydedup

import (
	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/v2/internal/dedup"
)

type MessageDeduplicator = dedup.ValkeyMessageDeduplicator
type NonceStore = dedup.ValkeyNonceStore

func NewMessageDeduplicator(valkeyClient valkey.Client) *MessageDeduplicator {
	return dedup.NewValkeyMessageDeduplicator(valkeyClient)
}

func NewNonceStore(valkeyClient valkey.Client) *NonceStore {
	return dedup.NewValkeyNonceStore(valkeyClient)
}
