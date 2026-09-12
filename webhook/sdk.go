package webhook

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NewSDKHandler는 옵션을 순서대로 한 번 적용한 뒤 SDK 인증 설정을 검증하고 handler를 활성화한다.
// WithWebhookToken이 비어 있으면 IRIS_WEBHOOK_TOKEN을 사용하며 secret만 지정해도 허용한다.
// Handler가 nil이면 옵션 실행 전에 거절하고, 인증·nonce 검증 실패에서는 worker를 시작하지 않는다.
// SDK 오류 문맥은 iris.NewWebhookHandler의 기존 반환 계약을 유지한다.
func NewSDKHandler(handler MessageHandler, opts ...HandlerOption) (*Handler, error) {
	if handler == nil {
		return nil, errors.New("iris: message handler is required")
	}

	result := newHandler("", handler, nil)
	result.applyOptions(opts)

	return result.initializeSDK("iris: new webhook handler")
}

// NewSDKDurableHandler는 SDK 설정을 한 번 적용한 durable admission 전용 handler를 만든다.
// Admitter가 nil이면 옵션 실행 전에 거절하고 명시적 admitter는 WithDurableAdmission보다 우선한다.
// 인증·context·오류 규칙은 NewSDKHandler와 같으며 메시지 실행은 소비자의 inbox가 소유한다.
func NewSDKDurableHandler(admitter MessageAdmitter, opts ...HandlerOption) (*Handler, error) {
	if admitter == nil {
		return nil, ErrMessageAdmitterRequired
	}

	result := newHandler("", nil, nil)
	result.applyOptions(opts)

	result.admitter = admitter

	return result.initializeSDK("iris: new durable webhook handler")
}

func (h *Handler) initializeSDK(errorContext string) (*Handler, error) {
	// 환경값은 옵션 실행 뒤에 읽어 기존 SDK의 설정 해석 순서를 보존한다.
	h.token = cmp.Or(strings.TrimSpace(h.sdkToken), strings.TrimSpace(os.Getenv("IRIS_WEBHOOK_TOKEN")))
	if h.token == "" && strings.TrimSpace(h.webhookSecret) == "" {
		return nil, errors.New("iris: webhook token or secret is required (set IRIS_WEBHOOK_TOKEN, webhook.WithWebhookToken, or webhook.WithWebhookSecret)")
	}

	h.logger = resolveLogger(h.sdkLogger)

	ctx := cmp.Or(h.sdkCtx, context.Background())

	result, err := h.initialize(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorContext, err)
	}

	return result, nil
}

func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.sdkToken = token
	}
}

func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.sdkLogger = logger
	}
}

func WithContext(ctx context.Context) HandlerOption {
	return func(h *Handler) {
		h.sdkCtx = ctx //nolint:fatcontext // 옵션으로 받은 context를 보관만 하고 파생하지 않는다.
	}
}

type SDKConfig struct {
	Token  string
	Secret string
	Logger *slog.Logger
	Ctx    context.Context //nolint:containedctx // WithContext 옵션 값을 그대로 노출하는 SDK 설정 스냅샷이다.
}

// ResolveSDKConfig는 zero Handler에 옵션을 독립적으로 한 번 적용한 SDK 설정 snapshot을 반환한다.
// 환경값 해석·검증·worker 활성화는 수행하지 않으며 생성자 호출의 옵션 적용을 대신하지 않는다.
func ResolveSDKConfig(opts []HandlerOption) SDKConfig {
	var h Handler

	h.applyOptions(opts)

	return SDKConfig{Token: h.sdkToken, Secret: h.webhookSecret, Logger: h.sdkLogger, Ctx: h.sdkCtx}
}
