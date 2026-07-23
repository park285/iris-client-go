package iris

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	client "github.com/park285/iris-client-go/internal/client/transport"
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

func NewWebhookHandler(handler basewebhook.MessageHandler, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	if handler == nil {
		return nil, errors.New("iris: message handler is required")
	}

	ctx, token, logger, err := resolveWebhookSDKParams(opts)
	if err != nil {
		return nil, err
	}

	return basewebhook.NewHandler(ctx, token, handler, logger, opts...), nil
}

func NewDurableWebhookHandler(admitter basewebhook.MessageAdmitter, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	if admitter == nil {
		return nil, basewebhook.ErrMessageAdmitterRequired
	}

	ctx, token, logger, err := resolveWebhookSDKParams(opts)
	if err != nil {
		return nil, err
	}

	return basewebhook.NewDurableHandler(ctx, token, admitter, logger, opts...)
}

func resolveWebhookSDKParams(opts []basewebhook.HandlerOption) (context.Context, string, *slog.Logger, error) {
	cfg := basewebhook.ResolveSDKConfig(opts)

	token := firstNonEmpty(cfg.Token, os.Getenv(EnvWebhookToken))
	secret := firstNonEmpty(cfg.Secret)
	if token == "" && secret == "" {
		return nil, "", nil, errors.New("iris: webhook token or secret is required (set IRIS_WEBHOOK_TOKEN, webhook.WithWebhookToken, or webhook.WithWebhookSecret)")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return ctx, token, logger, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
