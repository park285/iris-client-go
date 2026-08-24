package iris

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	client "github.com/park285/iris-client-go/v2/internal/client/transport"
	basewebhook "github.com/park285/iris-client-go/v2/webhook"
)

const (
	EnvBaseURL      = "IRIS_BASE_URL"
	EnvBotToken     = "IRIS_BOT_TOKEN"     // #nosec G101 -- 자격증명이 아니라 환경 변수 이름이다.
	EnvWebhookToken = "IRIS_WEBHOOK_TOKEN" // #nosec G101 -- 자격증명이 아니라 환경 변수 이름이다.
)

func NewClient(opts ...ClientOption) (*APIClient, error) {
	cfg := client.ResolveSDKConfig(opts)

	baseURL := firstNonEmpty(cfg.BaseURL, os.Getenv(EnvBaseURL))
	if baseURL == "" {
		return nil, errors.New("iris: base URL is required (set IRIS_BASE_URL or use WithBaseURL)")
	}

	botToken := firstNonEmpty(cfg.BotToken, os.Getenv(EnvBotToken))
	if botToken == "" {
		return nil, errors.New("iris: bot token is required (set IRIS_BOT_TOKEN or use WithBotToken)")
	}

	irisClient := NewAPIClient(baseURL, botToken, opts...)
	if irisClient.InitError() != nil {
		return nil, irisClient.InitError() //nolint:wrapcheck // InitError는 transport 초기화 오류를 그대로 노출하는 공개 계약이다.
	}

	return irisClient, nil
}

func NewWebhookHandler(handler basewebhook.MessageHandler, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	if handler == nil {
		return nil, errors.New("iris: message handler is required")
	}

	ctx, token, logger, err := resolveWebhookSDKParams(opts)
	if err != nil {
		return nil, err //nolint:wrapcheck // 검증 오류가 필드 맥락을 이미 담고 있어 그대로 전달한다.
	}

	return basewebhook.NewHandler(ctx, token, handler, logger, opts...) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
}

func NewDurableWebhookHandler(admitter basewebhook.MessageAdmitter, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	if admitter == nil {
		return nil, basewebhook.ErrMessageAdmitterRequired
	}

	ctx, token, logger, err := resolveWebhookSDKParams(opts)
	if err != nil {
		return nil, err //nolint:wrapcheck // 검증 오류가 필드 맥락을 이미 담고 있어 그대로 전달한다.
	}

	return basewebhook.NewDurableHandler(ctx, token, admitter, logger, opts...) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
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
