package client

import (
	"testing"
	"time"
)

func TestResolveSDKConfig(t *testing.T) {
	t.Parallel()

	t.Run("extracts base URL and bot token from options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]ClientOption{
			WithBaseURL("http://test:3000"),
			WithBotToken("test-token"),
		})
		if cfg.BaseURL != "http://test:3000" {
			t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://test:3000")
		}
		if cfg.BotToken != "test-token" {
			t.Fatalf("BotToken = %q, want %q", cfg.BotToken, "test-token")
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]ClientOption{WithTimeout(5 * time.Second)})
		if cfg.BaseURL != "" || cfg.BotToken != "" {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		cfg := ResolveSDKConfig(nil)
		if cfg.BaseURL != "" || cfg.BotToken != "" {
			t.Fatal("expected zero SDK config")
		}
	})
}
