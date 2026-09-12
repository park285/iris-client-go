package iris

import (
	"errors"
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
		return nil, irisClient.InitError()
	}

	return irisClient, nil
}

// NewWebhookHandler는 옵션을 한 번 적용하고 SDK 인증 설정을 검증한 뒤 webhook 처리를 시작한다.
func NewWebhookHandler(handler basewebhook.MessageHandler, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	return basewebhook.NewSDKHandler(handler, opts...) //nolint:wrapcheck // SDK 생성 owner가 기존 iris 오류 문맥을 제공하므로 추가 래핑은 공개 오류 계약을 바꾼다.
}

// NewDurableWebhookHandler는 옵션을 한 번 적용한 durable admission 전용 handler를 만든다.
// 명시적 admitter는 WithDurableAdmission 옵션보다 우선하며 메시지 실행은 소비자가 소유한다.
func NewDurableWebhookHandler(admitter basewebhook.MessageAdmitter, opts ...basewebhook.HandlerOption) (*basewebhook.Handler, error) {
	return basewebhook.NewSDKDurableHandler(admitter, opts...) //nolint:wrapcheck // SDK 생성 owner의 오류 문맥과 nil admitter sentinel 반환을 그대로 보존한다.
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}

	return ""
}
