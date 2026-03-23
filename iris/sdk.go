package iris

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/park285/iris-client-go/client"
	"github.com/park285/iris-client-go/dedup"
	basewebhook "github.com/park285/iris-client-go/webhook"
	"github.com/valkey-io/valkey-go"
)

const (
	// EnvBaseURL is the environment variable for the Iris server URL.
	EnvBaseURL = "IRIS_BASE_URL"
	// EnvBotToken is the environment variable for the bot authentication token.
	EnvBotToken = "IRIS_BOT_TOKEN"
	// EnvWebhookToken is the environment variable for the webhook authentication token.
	EnvWebhookToken = "IRIS_WEBHOOK_TOKEN"
)

// NewClient creates an Iris client.
// Base URL and bot token are read from IRIS_BASE_URL and IRIS_BOT_TOKEN
// environment variables. Use WithBaseURL/WithBotToken to override.
func NewClient(opts ...ClientOption) (*H2CClient, error) {
	cfg := client.ResolveSDKConfig(opts)

	baseURL := firstNonEmpty(cfg.BaseURL, os.Getenv(EnvBaseURL))
	if baseURL == "" {
		return nil, errors.New("iris: base URL is required (set IRIS_BASE_URL or use WithBaseURL)")
	}

	botToken := firstNonEmpty(cfg.BotToken, os.Getenv(EnvBotToken))
	if botToken == "" {
		return nil, errors.New("iris: bot token is required (set IRIS_BOT_TOKEN or use WithBotToken)")
	}

	return NewH2CClient(baseURL, botToken, opts...), nil
}

// NewWebhookHandler creates an Iris webhook handler.
// Webhook token is read from IRIS_WEBHOOK_TOKEN. Use WithWebhookToken to override.
// Logger defaults to slog.Default(). Context defaults to context.Background().
func NewWebhookHandler(handler MessageHandler, opts ...HandlerOption) (*WebhookHandler, error) {
	if handler == nil {
		return nil, errors.New("iris: message handler is required")
	}

	cfg := basewebhook.ResolveSDKConfig(opts)

	token := firstNonEmpty(cfg.Token, os.Getenv(EnvWebhookToken))
	if token == "" {
		return nil, errors.New("iris: webhook token is required (set IRIS_WEBHOOK_TOKEN or use WithWebhookToken)")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return NewHandler(ctx, token, handler, logger, opts...), nil
}

// NewValkeyDeduplicator creates a Valkey-backed deduplicator.
var NewValkeyDeduplicator = dedup.NewValkeyDeduplicator

// ValkeyDeduplicator is the Valkey dedup implementation type.
type ValkeyDeduplicator = dedup.ValkeyDeduplicator

// WithValkeyDedup configures the webhook handler to use Valkey deduplication.
func WithValkeyDedup(valkeyClient valkey.Client) HandlerOption {
	return WithDeduplicator(NewValkeyDeduplicator(valkeyClient))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
