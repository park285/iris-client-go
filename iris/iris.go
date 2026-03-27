package iris

import (
	"context"
	"log/slog"

	"github.com/park285/iris-client-go/client"
	basewebhook "github.com/park285/iris-client-go/webhook"
)

// H2CClient는 iris-client-go의 기본 클라이언트 타입 별칭입니다.
type H2CClient = client.H2CClient

// Sender는 메시지 발신 전용 인터페이스입니다.
type Sender = client.Sender

// AdminClient는 Iris 관리/유틸 API 인터페이스입니다.
type AdminClient = client.AdminClient

// ClientOption은 클라이언트 생성 옵션입니다.
type ClientOption = client.ClientOption

// SendOption은 메시지 발신 옵션입니다.
type SendOption = client.SendOption

// PingStrategy는 ping probe 전략입니다.
type PingStrategy = client.PingStrategy

// ReplyRequest는 reply 요청 DTO입니다.
type ReplyRequest = client.ReplyRequest

// ConfigResponse는 Iris 설정 응답 DTO입니다.
type ConfigResponse = client.ConfigResponse

// DecryptRequest는 decrypt 요청 DTO입니다.
type DecryptRequest = client.DecryptRequest

// DecryptResponse는 decrypt 응답 DTO입니다.
type DecryptResponse = client.DecryptResponse

// WebhookHandler는 Iris webhook handler 타입입니다.
type WebhookHandler = basewebhook.Handler

// HandlerOption은 webhook handler 옵션입니다.
type HandlerOption = basewebhook.HandlerOption

// MessageHandler는 webhook 메시지 소비 인터페이스입니다.
type MessageHandler = basewebhook.MessageHandler

// Message는 webhook 표준 메시지 타입입니다.
type Message = basewebhook.Message

// MessageJSON은 webhook message JSON payload 타입입니다.
type MessageJSON = basewebhook.MessageJSON

// WebhookRequest는 webhook 요청 DTO입니다.
type WebhookRequest = basewebhook.WebhookRequest

// Metrics는 webhook metrics 인터페이스입니다.
type Metrics = basewebhook.Metrics

// Deduplicator는 webhook dedup 인터페이스입니다.
type Deduplicator = basewebhook.Deduplicator

// Client는 봇 코드가 공통으로 의존할 Iris 상위 인터페이스입니다.
type Client interface {
	Sender
	AdminClient
}

const (
	PathReply      = client.PathReply
	PathReplyImage = client.PathReplyImage
	PathReady      = client.PathReady
	PathHealth  = client.PathHealth
	PathConfig  = client.PathConfig
	PathDecrypt = client.PathDecrypt
	PathWebhook = basewebhook.PathWebhook

	HeaderBotToken      = client.HeaderBotToken
	HeaderIrisToken     = basewebhook.HeaderIrisToken
	HeaderIrisMessageID = basewebhook.HeaderIrisMessageID

	PingStrategyAuto   = client.PingStrategyAuto
	PingStrategyReady  = client.PingStrategyReady
	PingStrategyHealth = client.PingStrategyHealth

	DefaultDedupTTL = basewebhook.DefaultDedupTTL
)

var (
	WithTransport             = client.WithTransport
	WithTimeout               = client.WithTimeout
	WithDialTimeout           = client.WithDialTimeout
	WithTLSHandshakeTimeout   = client.WithTLSHandshakeTimeout
	WithResponseHeaderTimeout = client.WithResponseHeaderTimeout
	WithIdleConnTimeout       = client.WithIdleConnTimeout
	WithMaxIdleConns          = client.WithMaxIdleConns
	WithMaxIdleConnsPerHost   = client.WithMaxIdleConnsPerHost
	WithMaxConnsPerHost       = client.WithMaxConnsPerHost
	WithReadIdleTimeout       = client.WithReadIdleTimeout
	WithPingTimeout           = client.WithPingTimeout
	WithPingProbeTimeout      = client.WithPingProbeTimeout
	WithPingStrategy          = client.WithPingStrategy
	WithWriteByteTimeout      = client.WithWriteByteTimeout
	WithLogger                = client.WithLogger
	WithHTTPClient            = client.WithHTTPClient
	WithRoundTripper          = client.WithRoundTripper
	WithReplyRetry            = client.WithReplyRetry
	WithBaseURL               = client.WithBaseURL
	WithBotToken              = client.WithBotToken
	WithThreadID              = client.WithThreadID
	WithThreadScope           = client.WithThreadScope

	WithWebhookToken    = basewebhook.WithWebhookToken
	WithWebhookLogger   = basewebhook.WithWebhookLogger
	WithContext         = basewebhook.WithContext
	WithMetrics         = basewebhook.WithMetrics
	WithDeduplicator    = basewebhook.WithDeduplicator
	WithWorkerCount     = basewebhook.WithWorkerCount
	WithQueueSize       = basewebhook.WithQueueSize
	WithEnqueueTimeout  = basewebhook.WithEnqueueTimeout
	WithHandlerTimeout  = basewebhook.WithHandlerTimeout
	WithRequireHTTP2    = basewebhook.WithRequireHTTP2
	WithDedupTTL        = basewebhook.WithDedupTTL
	WithDedupTimeout    = basewebhook.WithDedupTimeout
	WithMaxBodyBytes    = basewebhook.WithMaxBodyBytes
	WithAutoWorkerCount = basewebhook.WithAutoWorkerCount
	ResolveThreadID     = basewebhook.ResolveThreadID
	DedupKey            = basewebhook.DedupKey
)

// NewH2CClient는 iris-client-go 기반 클라이언트를 생성합니다.
func NewH2CClient(baseURL, botToken string, opts ...ClientOption) *H2CClient {
	return client.NewH2CClient(baseURL, botToken, opts...)
}

// NewHandler는 Iris webhook handler를 생성합니다.
func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *WebhookHandler {
	return basewebhook.NewHandler(ctx, token, handler, logger, opts...)
}
