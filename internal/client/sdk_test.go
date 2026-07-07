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
			WithTransport("http1"),
		})
		if cfg.BaseURL != "http://test:3000" {
			t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://test:3000")
		}
		if cfg.BotToken != "test-token" {
			t.Fatalf("BotToken = %q, want %q", cfg.BotToken, "test-token")
		}
		if cfg.Transport != "http1" {
			t.Fatalf("Transport = %q, want %q", cfg.Transport, "http1")
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]ClientOption{WithTimeout(5 * time.Second)})
		if cfg.BaseURL != "" || cfg.BotToken != "" || cfg.Transport != "" {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		cfg := ResolveSDKConfig(nil)
		if cfg.BaseURL != "" || cfg.BotToken != "" || cfg.Transport != "" {
			t.Fatal("expected zero SDK config")
		}
	})
}
