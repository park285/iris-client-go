package webhook

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

func newTestHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	merged := make([]HandlerOption, 0, len(opts)+1)
	merged = append(merged, WithNonceStore(newMemoryNonceCache()))
	merged = append(merged, opts...)

	result, err := NewHandler(ctx, token, handler, logger, merged...)
	if err != nil {
		panic(err)
	}

	return result
}

func TestNewHandlerRequiresExplicitSetOnceNonceStore(t *testing.T) {
	t.Parallel()

	result, err := NewHandler(t.Context(), "token", &captureHandler{msgCh: make(chan *Message, 1)}, slog.Default())
	if result != nil {
		t.Fatal("NewHandler() returned a handler without a nonce store")
	}
	if !errors.Is(err, ErrNonceStoreRequired) {
		t.Fatalf("NewHandler() error = %v, want %v", err, ErrNonceStoreRequired)
	}
}
