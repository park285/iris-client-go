package webhook

import (
	"context"
	"testing"
	"time"
)

type publicAPIV2NonceStore struct{}

func (publicAPIV2NonceStore) IsDuplicate(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func (publicAPIV2NonceStore) SetOnceNonce() {}

type publicAPIV2MessageDeduplicator struct{}

func (publicAPIV2MessageDeduplicator) Reserve(context.Context, string, time.Duration) (string, DedupState, error) {
	return "token", DedupStateReserved, nil
}

func (publicAPIV2MessageDeduplicator) Commit(context.Context, string, string, time.Duration) error {
	return nil
}

func (publicAPIV2MessageDeduplicator) ReleaseReservation(context.Context, string, string) error {
	return nil
}

func TestV2PublicAPIRolesAreSeparate(t *testing.T) {
	t.Parallel()

	var nonceStore SetOnceNonceStore = publicAPIV2NonceStore{}
	var messageDeduplicator MessageDeduplicator = publicAPIV2MessageDeduplicator{}
	_ = WithNonceStore(nonceStore)
	_ = WithMessageDeduplicator(messageDeduplicator)
}
