package valkeydedup_test

import (
	"testing"

	"github.com/park285/iris-client-go/v3/valkeydedup"
	"github.com/park285/iris-client-go/v3/webhook"
)

func TestConstructorsExposeSeparateRoles(t *testing.T) {
	t.Parallel()

	messageDeduplicator := valkeydedup.NewMessageDeduplicator(nil)
	nonceStore := valkeydedup.NewNonceStore(nil)

	if messageDeduplicator == nil || nonceStore == nil {
		t.Fatal("valkeydedup constructors returned nil")
	}

	var (
		_ webhook.MessageDeduplicator = messageDeduplicator
		_ webhook.SetOnceNonceStore   = nonceStore
	)
}
