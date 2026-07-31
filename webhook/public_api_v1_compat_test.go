package webhook

import "testing"

func TestV125PublicAPICompatibility(t *testing.T) {
	t.Parallel()

	var legacyNonceOption func(Deduplicator) HandlerOption = WithNonceCache
	_ = legacyNonceOption
	_ = HandlerOptions{0, 0, 0, 0, 0, 0, 0, 0, 0}
	_ = ReceiveDiagnostics{0, 0, 0, 0, 0, 0, 0}

	var nonceStore NonceStore = NoopDeduplicator{}
	_ = WithNonceCache(nonceStore)
}
