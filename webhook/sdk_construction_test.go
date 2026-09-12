package webhook

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type constructionContextKey struct{}

func sdkConstructionCases() []struct {
	name      string
	construct func(...HandlerOption) (*Handler, error)
} {
	return []struct {
		name      string
		construct func(...HandlerOption) (*Handler, error)
	}{
		{"message", func(opts ...HandlerOption) (*Handler, error) {
			return NewSDKHandler(&captureHandler{}, opts...)
		}},
		{"durable", func(opts ...HandlerOption) (*Handler, error) {
			return NewSDKDurableHandler(&recordingAdmitter{}, opts...)
		}},
	}
}

func TestSDKConstructionPreservesFinalOptionsAndContext(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "environment-token")

	for _, test := range sdkConstructionCases() {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), constructionContextKey{}, "startup"))
			cancel()

			logger := slog.New(slog.DiscardHandler)
			nonces := newMemoryNonceCache()
			calls := 0

			handler, err := test.construct(
				WithWebhookToken("old-token"), WithWebhookToken(" final-token "),
				WithWebhookSecret(" old-secret "), WithWebhookSecret(" final-secret "),
				WithWebhookLogger(nil), WithWebhookLogger(logger),
				WithContext(t.Context()), WithContext(ctx),
				WithWorkerCount(1), WithWorkerCount(2),
				WithNonceStore(newMemoryNonceCache()), WithNonceStore(nonces),
				nil, func(*Handler) { calls++ },
			)
			if err != nil {
				t.Fatalf("construct SDK handler: %v", err)
			}

			t.Cleanup(func() { closeHandler(t, handler) })

			if calls != 1 || handler.token != "final-token" || handler.webhookSecret != "final-secret" {
				t.Fatalf("calls/token/secret = %d/%q/%q", calls, handler.token, handler.webhookSecret)
			}

			if handler.logger != logger || handler.nonceStore != nonces || handler.options.WorkerCount != 2 {
				t.Fatal("last logger, nonce store or worker count was not applied")
			}

			if handler.runCtx.Value(constructionContextKey{}) != "startup" || handler.runCtx.Err() != nil {
				t.Fatal("startup values must survive without inheriting startup cancellation")
			}

			closeHandler(t, handler)

			if !errors.Is(handler.runCtx.Err(), context.Canceled) {
				t.Fatal("close must cancel the handler lifetime context")
			}
		})
	}
}

func TestSDKConstructionFinalBlankAndNilRestoreDefaults(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", " environment-token ")

	for _, test := range sdkConstructionCases() {
		t.Run(test.name, func(t *testing.T) {
			var defaultCtx context.Context

			previousCtx := context.WithValue(t.Context(), constructionContextKey{}, "previous")
			previousLogger := slog.New(slog.DiscardHandler)
			defaultLogger := slog.Default()
			calls := 0

			handler, err := test.construct(
				WithWebhookToken("old-token"), WithWebhookToken(" \t "),
				WithWebhookLogger(previousLogger), WithWebhookLogger(nil),
				WithContext(previousCtx), WithContext(defaultCtx),
				WithNonceStore(newMemoryNonceCache()),
				func(*Handler) { calls++ },
			)
			if err != nil {
				t.Fatalf("construct SDK handler: %v", err)
			}

			t.Cleanup(func() { closeHandler(t, handler) })

			if calls != 1 {
				t.Fatalf("option calls = %d, want 1", calls)
			}

			if handler.token != "environment-token" || handler.webhookSecret != "environment-token" {
				t.Fatalf("token/secret = %q/%q, want environment-token", handler.token, handler.webhookSecret)
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
		})
	}
}

func TestSDKConstructionAuthSources(t *testing.T) {
	for _, test := range []struct {
		name   string
		env    string
		token  string
		secret string
		want   string
	}{
		{"environment", " env-token ", "", "", "env-token"},
		{"blank option uses environment", "env-token", "  ", "", "env-token"},
		{"token option", "env-token", " option-token ", "", "option-token"},
		{"secret only", "", "", " secret-only ", "secret-only"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("IRIS_WEBHOOK_TOKEN", test.env)

			for _, constructor := range sdkConstructionCases() {
				t.Run(constructor.name, func(t *testing.T) {
					handler, err := constructor.construct(WithWebhookToken(test.token), WithWebhookSecret(test.secret), WithNonceStore(newMemoryNonceCache()))
					if err != nil {
						t.Fatalf("construct SDK handler: %v", err)
					}

					t.Cleanup(func() { closeHandler(t, handler) })

					if handler.webhookSecret != test.want || handler.logger != slog.Default() || handler.runCtx == nil {
						t.Fatalf("unexpected SDK defaults: secret=%q", handler.webhookSecret)
					}
				})
			}
		})
	}
}

func TestSDKConstructionFailureOrderDoesNotStartWorkers(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "")

	for _, test := range sdkConstructionCases() {
		for _, token := range []string{"", "valid-token"} {
			t.Run(test.name+"/"+token, func(t *testing.T) {
				calls := 0

				var configured *Handler

				handler, err := test.construct(WithWebhookToken(token), func(h *Handler) {
					calls++

					configured = h
				})
				if err == nil || handler != nil || calls != 1 {
					t.Fatalf("failure result: handler=%p err=%v calls=%d", handler, err, calls)
				}

				if configured.sched != nil || configured.taskPool != nil || configured.runCancel != nil {
					t.Fatal("failed construction activated worker resources")
				}

				if token == "" {
					if !strings.HasPrefix(err.Error(), "iris: webhook token or secret is required") || errors.Is(err, ErrNonceStoreRequired) {
						t.Fatalf("missing auth must win over missing nonce: %v", err)
					}
				} else if !errors.Is(err, ErrNonceStoreRequired) {
					t.Fatalf("expected wrapped nonce error: %v", err)
				}
			})
		}
	}
}

func TestSDKConstructionNilInputSkipsOptions(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "")

	calls := 0
	option := func(*Handler) { calls++ }

	if _, err := NewSDKHandler(nil, option); err == nil || err.Error() != "iris: message handler is required" {
		t.Fatalf("nil message handler error = %v", err)
	}

	if _, err := NewSDKDurableHandler(nil, option); !errors.Is(err, ErrMessageAdmitterRequired) {
		t.Fatalf("nil admitter error = %v", err)
	}

	if calls != 0 {
		t.Fatalf("nil input applied options %d times", calls)
	}
}

func TestDirectConstructionIgnoresSDKOverrides(t *testing.T) {
	t.Parallel()

	for _, durable := range []bool{false, true} {
		name := "message"

		if durable {
			name = "durable"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.WithValue(t.Context(), constructionContextKey{}, "explicit")
			logger := slog.New(slog.DiscardHandler)
			calls := 0
			opts := []HandlerOption{
				WithWebhookToken("ignored-token"), WithWebhookLogger(slog.New(slog.DiscardHandler)),
				WithContext(context.WithValue(t.Context(), constructionContextKey{}, "ignored")),
				WithNonceStore(newMemoryNonceCache()), nil, func(*Handler) { calls++ },
			}

			var (
				handler *Handler
				err     error
			)

			if durable {
				handler, err = NewDurableHandler(ctx, " explicit-token ", &recordingAdmitter{}, logger, opts...)
			} else {
				handler, err = NewHandler(ctx, " explicit-token ", &captureHandler{}, logger, opts...)
			}

			if err != nil {
				t.Fatalf("construct direct handler: %v", err)
			}

			t.Cleanup(func() { closeHandler(t, handler) })

			if calls != 1 || handler.token != "explicit-token" || handler.webhookSecret != "explicit-token" || handler.logger != logger {
				t.Fatal("SDK-only options changed explicit constructor arguments")
			}

			if handler.runCtx.Value(constructionContextKey{}) != "explicit" {
				t.Fatal("direct constructor did not retain its explicit context")
			}
		})
	}
}

func TestDurableConstructionExplicitAdmitterWins(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "test-token")

	for _, sdk := range []bool{false, true} {
		explicit := &recordingAdmitter{}
		override := &recordingAdmitter{}
		opts := []HandlerOption{WithNonceStore(newMemoryNonceCache()), WithDurableAdmission(override)}

		var (
			handler *Handler
			err     error
		)

		if sdk {
			handler, err = NewSDKDurableHandler(explicit, opts...)
		} else {
			handler, err = NewDurableHandler(t.Context(), "test-token", explicit, nil, opts...)
		}

		if err != nil {
			t.Fatalf("construct durable handler (SDK=%t): %v", sdk, err)
		}

		t.Cleanup(func() { closeHandler(t, handler) })

		if handler.admitter != explicit || handler.sched != nil || handler.taskPool != nil {
			t.Fatalf("durable constructor replaced explicit admitter or started a scheduler (SDK=%t)", sdk)
		}
	}
}

func TestResolveSDKConfigRemainsIndependentZeroSnapshot(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "not-a-default")

	calls := 0
	option := func(h *Handler) {
		calls++

		if h.options != (HandlerOptions{}) || h.logger != nil || h.closedCh != nil || h.runCtx != nil {
			t.Fatal("ResolveSDKConfig did not apply the option to a zero Handler")
		}
	}

	for range 2 {
		cfg := ResolveSDKConfig([]HandlerOption{nil, option, WithWebhookToken(" raw-token "), WithWebhookSecret(" secret ")})
		if cfg.Token != " raw-token " || cfg.Secret != "secret" || cfg.Logger != nil || cfg.Ctx != nil {
			t.Fatalf("unexpected independent SDK snapshot: %#v", cfg)
		}
	}

	if calls != 2 {
		t.Fatalf("two independent resolutions applied options %d times", calls)
	}

	if cfg := ResolveSDKConfig(nil); cfg != (SDKConfig{}) {
		t.Fatalf("environment must not populate ResolveSDKConfig: %#v", cfg)
	}
}
