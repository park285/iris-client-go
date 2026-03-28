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
	// EnvBaseURL은 Iris 서버 URL 환경 변수입니다.
	EnvBaseURL = "IRIS_BASE_URL"
	// EnvBotToken은 봇 인증 토큰 환경 변수입니다.
	EnvBotToken = "IRIS_BOT_TOKEN"
	// EnvWebhookToken은 webhook 인증 토큰 환경 변수입니다.
	EnvWebhookToken = "IRIS_WEBHOOK_TOKEN"
)

// NewClient는 Iris 클라이언트를 생성합니다.
// Base URL과 봇 토큰은 IRIS_BASE_URL, IRIS_BOT_TOKEN 환경 변수에서 읽습니다.
// WithBaseURL/WithBotToken으로 재정의할 수 있습니다.
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

// NewWebhookHandler는 Iris webhook 핸들러를 생성합니다.
// Webhook 토큰은 IRIS_WEBHOOK_TOKEN에서 읽습니다. WithWebhookToken으로 재정의할 수 있습니다.
// Logger 기본값은 slog.Default(), Context 기본값은 context.Background()입니다.
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

// NewValkeyDeduplicator는 Valkey 기반 중복 제거기를 생성합니다.
var NewValkeyDeduplicator = dedup.NewValkeyDeduplicator

type ValkeyDeduplicator = dedup.ValkeyDeduplicator

// WithValkeyDedup은 webhook 핸들러에 Valkey 중복 제거를 설정합니다.
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
