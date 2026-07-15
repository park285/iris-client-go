package webhook

import "testing"

func TestWithDedupModeBeforeDecodeNormalizesToSafePolicy(t *testing.T) {
	t.Parallel()

	handler := &Handler{options: defaultHandlerOptions()}
	WithDedupMode(DedupModeBeforeDecode)(handler)

	if handler.options.DedupMode != DedupModeAfterDecode {
		t.Fatalf("DedupModeBeforeDecode normalized to %v, want DedupModeAfterDecode", handler.options.DedupMode)
	}
}
