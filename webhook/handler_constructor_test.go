package webhook

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// newTestHandler는 테스트 token, test 수명 context, 메모리 set-once nonce store를 기본값으로 durable
// handler를 만든다. 뒤에 오는 opts가 기본값을 덮는다.
func newTestHandler(t *testing.T, admitter MessageAdmitter, opts ...HandlerOption) *Handler {
	t.Helper()

	defaults := []HandlerOption{
		WithWebhookToken(testToken),
		WithContext(t.Context()),
		WithNonceStore(newMemoryNonceCache()),
	}

	handler, err := NewDurableHandler(admitter, append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("NewDurableHandler() error = %v", err)
	}

	return handler
}

func mustNewDurableHandler(t *testing.T, admitter MessageAdmitter, logger *slog.Logger, opts ...HandlerOption) *Handler {
	t.Helper()

	return newTestHandler(t, admitter, append([]HandlerOption{WithWebhookLogger(logger)}, opts...)...)
}

// withOneMinuteReplayWindow는 공개 옵션이 없는 replay window를 패키지 내부 테스트에서만 1분으로 줄인다.
func withOneMinuteReplayWindow() HandlerOption {
	return func(h *Handler) {
		h.replayWindow = time.Minute
	}
}

type constructionContextKey struct{}

type markerlessNonceStore struct{}

func (markerlessNonceStore) IsDuplicate(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func TestNewDurableHandlerAppliesFinalOptionsAndContext(t *testing.T) {
	t.Setenv(envWebhookToken, "environment-token")

	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), constructionContextKey{}, "startup"))
	cancel()

	logger := slog.New(slog.DiscardHandler)
	nonces := newMemoryNonceCache()
	calls := 0

	handler, err := NewDurableHandler(&recordingAdmitter{},
		WithWebhookToken("old-token"), WithWebhookToken(" final-token "),
		WithWebhookLogger(nil), WithWebhookLogger(logger),
		WithContext(t.Context()), WithContext(ctx),
		WithAdmitTimeout(time.Second), WithAdmitTimeout(2*time.Second),
		WithNonceTimeout(time.Second), WithNonceTimeout(3*time.Second),
		WithMaxBodyBytes(1), WithMaxBodyBytes(4096),
		WithNonceStore(newMemoryNonceCache()), WithNonceStore(nonces),
		nil, func(*Handler) { calls++ },
	)
	if err != nil {
		t.Fatalf("NewDurableHandler() error = %v", err)
	}

	t.Cleanup(func() { closeHandler(t, handler) })

	if calls != 1 || handler.token != "final-token" {
		t.Fatalf("calls/token = %d/%q", calls, handler.token)
	}

	if handler.logger != logger || handler.nonceStore != nonces {
		t.Fatal("last logger or nonce store was not applied")
	}

	if got := handler.options; got != (handlerOptions{admitTimeout: 2 * time.Second, nonceTimeout: 3 * time.Second, maxBodyBytes: 4096}) {
		t.Fatalf("options = %+v, want the last value of each option", got)
	}

	if handler.runCtx.Value(constructionContextKey{}) != "startup" || handler.runCtx.Err() != nil {
		t.Fatal("startup values must survive without inheriting startup cancellation")
	}

	closeHandler(t, handler)

	if !errors.Is(handler.runCtx.Err(), context.Canceled) {
		t.Fatal("close must cancel the handler lifetime context")
	}
}

func TestNewDurableHandlerFinalBlankAndNilRestoreDefaults(t *testing.T) {
	t.Setenv(envWebhookToken, " environment-token ")

	var defaultCtx context.Context

	previousCtx := context.WithValue(t.Context(), constructionContextKey{}, "previous")
	previousLogger := slog.New(slog.DiscardHandler)
	defaultLogger := slog.Default()

	handler, err := NewDurableHandler(&recordingAdmitter{},
		WithWebhookToken("old-token"), WithWebhookToken(" \t "),
		WithWebhookLogger(previousLogger), WithWebhookLogger(nil),
		WithContext(previousCtx), WithContext(defaultCtx),
		WithAdmitTimeout(0), WithNonceTimeout(-time.Second), WithMaxBodyBytes(0),
		WithNonceStore(newMemoryNonceCache()),
	)
	if err != nil {
		t.Fatalf("NewDurableHandler() error = %v", err)
	}

	t.Cleanup(func() { closeHandler(t, handler) })

	if handler.token != "environment-token" {
		t.Fatalf("token = %q, want environment-token", handler.token)
	}

	if handler.logger != defaultLogger {
		t.Fatal("final nil logger did not restore the default logger")
	}

	if got := handler.runCtx.Value(constructionContextKey{}); got != nil {
		t.Fatalf("final nil context retained previous value %v", got)
	}

	if handler.runCtx.Err() != nil {
		t.Fatalf("background handler context error = %v", handler.runCtx.Err())
	}

	if handler.options != defaultHandlerOptions() {
		t.Fatalf("options = %+v, want defaults for non-positive values", handler.options)
	}
}

func TestNewDurableHandlerTokenSources(t *testing.T) {
	for _, test := range []struct {
		name  string
		env   string
		token string
		want  string
	}{
		{"environment", " env-token ", "", "env-token"},
		{"blank option uses environment", "env-token", "  ", "env-token"},
		{"token option", "env-token", " option-token ", "option-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envWebhookToken, test.env)

			handler, err := NewDurableHandler(&recordingAdmitter{}, WithWebhookToken(test.token), WithNonceStore(newMemoryNonceCache()))
			if err != nil {
				t.Fatalf("NewDurableHandler() error = %v", err)
			}

			t.Cleanup(func() { closeHandler(t, handler) })

			if handler.token != test.want || handler.logger != slog.Default() || handler.runCtx == nil {
				t.Fatalf("unexpected defaults: token=%q", handler.token)
			}
		})
	}
}

func TestNewDurableHandlerFailureOrderDoesNotStartLifetime(t *testing.T) {
	for _, test := range []struct {
		name  string
		env   string
		token string
		store NonceStore
	}{
		{name: "missing token", token: ""},
		{name: "whitespace environment token", env: "   ", token: ""},
		{name: "missing nonce store", token: "valid-token"},
		{name: "nonce store without set-once marker", token: "valid-token", store: markerlessNonceStore{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envWebhookToken, test.env)

			calls := 0

			var configured *Handler

			handler, err := NewDurableHandler(&recordingAdmitter{}, WithWebhookToken(test.token), WithNonceStore(test.store), func(h *Handler) {
				calls++

				configured = h
			})
			if err == nil || handler != nil || calls != 1 {
				t.Fatalf("failure result: handler=%p err=%v calls=%d", handler, err, calls)
			}

			if configured.runCancel != nil || configured.webhookSigner != nil {
				t.Fatal("failed construction activated handler resources")
			}

			if strings.TrimSpace(test.token) == "" {
				if !strings.HasPrefix(err.Error(), "iris: webhook token is required") || errors.Is(err, ErrNonceStoreRequired) {
					t.Fatalf("missing auth must win over missing nonce: %v", err)
				}
			} else if !errors.Is(err, ErrNonceStoreRequired) || !strings.HasPrefix(err.Error(), "iris: new durable webhook handler: ") {
				t.Fatalf("expected wrapped nonce error: %v", err)
			}
		})
	}
}

func TestNewDurableHandlerRejectsNilAdmitterBeforeOptions(t *testing.T) {
	t.Setenv(envWebhookToken, "")

	calls := 0

	handler, err := NewDurableHandler(nil, func(*Handler) { calls++ })
	if !errors.Is(err, ErrMessageAdmitterRequired) {
		t.Fatalf("NewDurableHandler(nil admitter) error = %v, want %v even without a token", err, ErrMessageAdmitterRequired)
	}

	if handler != nil || calls != 0 {
		t.Fatalf("nil admitter result: handler=%p option calls=%d, want nil/0", handler, calls)
	}
}
