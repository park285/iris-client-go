package iris

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/internal/client"
	"github.com/park285/iris-client-go/internal/dedup"
	basewebhook "github.com/park285/iris-client-go/webhook"
)

const (
	EnvBaseURL      = "IRIS_BASE_URL"
	EnvBotToken     = "IRIS_BOT_TOKEN"
	EnvWebhookToken = "IRIS_WEBHOOK_TOKEN"
)

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

	irisClient := NewH2CClient(baseURL, botToken, opts...)
	if irisClient.InitError() != nil {
		return nil, irisClient.InitError()
	}

	return irisClient, nil
}

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

type ValkeyDeduplicator = dedup.ValkeyDeduplicator

func NewValkeyDeduplicator(valkeyClient valkey.Client) *ValkeyDeduplicator {
	return dedup.NewValkeyDeduplicator(valkeyClient)
}

func WithValkeyDedup(valkeyClient valkey.Client) HandlerOption {
	return WithDeduplicator(NewValkeyDeduplicator(valkeyClient))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
