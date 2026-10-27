package transport

import (
	"strings"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v3/internal/testsupport"
)

func TestResolveSDKConfig(t *testing.T) {
	t.Parallel()

	t.Run("extracts base URL and bot token from options", func(t *testing.T) {
		t.Parallel()

		cfg := ResolveSDKConfig([]ClientOption{
			WithBaseURL("http://test:3000"),
			WithBotToken("test-token"),
			WithTransport(transportHTTP1),
		})
		if cfg.BaseURL != "http://test:3000" {
			t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://test:3000")
		}

		if cfg.BotToken != "test-token" {
			t.Fatalf("BotToken = %q, want %q", cfg.BotToken, "test-token")
		}

		if cfg.Transport != transportHTTP1 {
			t.Fatalf("Transport = %q, want %q", cfg.Transport, transportHTTP1)
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		t.Parallel()

		cfg := ResolveSDKConfig([]ClientOption{WithTimeout(5 * time.Second)})
		if cfg.BaseURL != "" || cfg.BotToken != "" || cfg.Transport != "" {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		t.Parallel()

		cfg := ResolveSDKConfig(nil)
		if cfg.BaseURL != "" || cfg.BotToken != "" || cfg.Transport != "" {
			t.Fatal("expected zero SDK config")
		}
	})
}

func TestNewClientFromOptionsAppliesEachOptionOnce(t *testing.T) {
	t.Parallel()

	calls := 0
	countingOption := func(*clientOptions) { calls++ }

	c, err := NewClientFromOptions("http://fallback.invalid", "fallback-token", WithTransport(transportHTTP1), countingOption)
	if err != nil {
		t.Fatalf("NewClientFromOptions() error = %v", err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", c.Close)

	if calls != 1 {
		t.Fatalf("option calls = %d, want 1", calls)
	}
}

func TestNewClientFromOptionsResolvesOptionBeforeFallback(t *testing.T) {
	t.Parallel()

	c, err := NewClientFromOptions(" http://fallback.invalid ", "fallback-token",
		WithTransport(transportHTTP1),
		WithBaseURL(" http://option.invalid "),
		WithBotToken("  "),
	)
	if err != nil {
		t.Fatalf("NewClientFromOptions() error = %v", err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", c.Close)

	if c.baseURL != "http://option.invalid" {
		t.Fatalf("baseURL = %q, want option value", c.baseURL)
	}

	if c.signer == nil {
		t.Fatal("blank WithBotToken must fall back to the fallback token")
	}
}

func TestNewClientFromOptionsRejectsMissingValuesAndInitErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, baseURL, token string
		opts                 []ClientOption
		want                 string
	}{
		{name: "missing base URL", token: "token", want: "iris: base URL is required"},
		{name: "missing bot token", baseURL: "http://host.invalid", want: "iris: bot token is required"},
		{name: "init error", baseURL: "https://host.invalid", token: "token", opts: []ClientOption{WithTransport("h3"), WithH3CACertFile("/nonexistent/ca.pem")}, want: "build h3 transport"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewClientFromOptions(test.baseURL, test.token, test.opts...)
			if c != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewClientFromOptions() = %v, %v; want nil client and %q", c, err, test.want)
			}
		})
	}
}
