package webhook

import (
	"context"
	"log/slog"
	"testing"
)

func TestResolveSDKConfig(t *testing.T) {
	t.Parallel()

	t.Run("extracts token logger and context from options", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		ctx := context.Background()

		cfg := ResolveSDKConfig([]HandlerOption{
			WithWebhookToken("wh-token"),
			WithWebhookLogger(logger),
			WithContext(ctx),
		})
		if cfg.Token != "wh-token" {
			t.Fatalf("Token = %q, want %q", cfg.Token, "wh-token")
		}
		if cfg.Logger != logger {
			t.Fatal("Logger mismatch")
		}
		if cfg.Ctx != ctx {
			t.Fatal("Ctx mismatch")
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]HandlerOption{WithWorkerCount(8)})
		if cfg.Token != "" || cfg.Logger != nil || cfg.Ctx != nil {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		cfg := ResolveSDKConfig(nil)
		if cfg.Token != "" || cfg.Logger != nil || cfg.Ctx != nil {
			t.Fatal("expected zero SDK config")
		}
	})
}
