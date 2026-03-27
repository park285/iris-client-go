package iris

import (
	"context"
	"log/slog"

	"github.com/park285/iris-client-go/client"
	basewebhook "github.com/park285/iris-client-go/webhook"
)

// H2CClient는 iris-client-go의 기본 클라이언트 타입 별칭입니다.
type H2CClient = client.H2CClient

// --- Interfaces ---

type Sender = client.Sender
type AdminClient = client.AdminClient
type RoomClient = client.RoomClient
type EventStreamClient = client.EventStreamClient

// --- Options ---

type ClientOption = client.ClientOption
type SendOption = client.SendOption
type PingStrategy = client.PingStrategy
type RoomStatsOptions = client.RoomStatsOptions

// --- Request/Response DTOs ---

type ReplyRequest = client.ReplyRequest
type DecryptRequest = client.DecryptRequest
type DecryptResponse = client.DecryptResponse

// Config types
type ConfigResponse = client.ConfigResponse
type ConfigState = client.ConfigState
type ConfigDiscoveredState = client.ConfigDiscoveredState
type ConfigPendingRestart = client.ConfigPendingRestart
type ConfigUpdateRequest = client.ConfigUpdateRequest
type ConfigUpdateResponse = client.ConfigUpdateResponse

// Reply types
type ReplyAcceptedResponse = client.ReplyAcceptedResponse
type ReplyStatusSnapshot = client.ReplyStatusSnapshot

// Query types
type QueryRequest = client.QueryRequest
type QueryColumn = client.QueryColumn
type QueryResponse = client.QueryResponse

// Bridge types
type BridgeHealthResult = client.BridgeHealthResult
type BridgeHealthCheck = client.BridgeHealthCheck
type BridgeDiscoveryHook = client.BridgeDiscoveryHook

// Room types
type RoomListResponse = client.RoomListResponse
type RoomSummary = client.RoomSummary
type MemberListResponse = client.MemberListResponse
type MemberInfo = client.MemberInfo
type RoomInfoResponse = client.RoomInfoResponse
type NoticeInfo = client.NoticeInfo
type BotCommandInfo = client.BotCommandInfo
type OpenLinkInfo = client.OpenLinkInfo
type StatsResponse = client.StatsResponse
type PeriodRange = client.PeriodRange
type MemberStats = client.MemberStats
type MemberActivityResponse = client.MemberActivityResponse

// Event types
type MemberEvent = client.MemberEvent
type NicknameChangeEvent = client.NicknameChangeEvent
type RoleChangeEvent = client.RoleChangeEvent
type ProfileChangeEvent = client.ProfileChangeEvent
type RawSSEEvent = client.RawSSEEvent

// Webhook types
type WebhookHandler = basewebhook.Handler
type HandlerOption = basewebhook.HandlerOption
type MessageHandler = basewebhook.MessageHandler
type Message = basewebhook.Message
type MessageJSON = basewebhook.MessageJSON
type WebhookRequest = basewebhook.WebhookRequest
type Metrics = basewebhook.Metrics
type Deduplicator = basewebhook.Deduplicator

// Client는 봇 코드가 공통으로 의존할 Iris 상위 인터페이스입니다.
type Client interface {
	Sender
	AdminClient
}

// FullClient는 모든 Iris 기능을 포함하는 확장 인터페이스입니다.
type FullClient interface {
	Sender
	AdminClient
	RoomClient
	EventStreamClient
}

const (
	PathReply              = client.PathReply
	PathReplyImage         = client.PathReplyImage
	PathReplyMarkdown      = client.PathReplyMarkdown
	PathReplyStatus        = client.PathReplyStatus
	PathReady              = client.PathReady
	PathHealth             = client.PathHealth
	PathConfig             = client.PathConfig
	PathDecrypt            = client.PathDecrypt
	PathQuery              = client.PathQuery
	PathDiagnosticsBridge  = client.PathDiagnosticsBridge
	PathRooms              = client.PathRooms
	PathEventsStream       = client.PathEventsStream
	PathWebhook            = basewebhook.PathWebhook

	HeaderBotToken      = client.HeaderBotToken
	HeaderIrisTimestamp = client.HeaderIrisTimestamp
	HeaderIrisNonce     = client.HeaderIrisNonce
	HeaderIrisSignature = client.HeaderIrisSignature
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
	WithHMACSecret            = client.WithHMACSecret
	WithThreadID              = client.WithThreadID
	WithThreadScope           = client.WithThreadScope

	WithWebhookToken    = basewebhook.WithWebhookToken
	WithWebhookLogger   = basewebhook.WithWebhookLogger
	WithContext          = basewebhook.WithContext
	WithMetrics          = basewebhook.WithMetrics
	WithDeduplicator     = basewebhook.WithDeduplicator
	WithWorkerCount      = basewebhook.WithWorkerCount
	WithQueueSize        = basewebhook.WithQueueSize
	WithEnqueueTimeout   = basewebhook.WithEnqueueTimeout
	WithHandlerTimeout   = basewebhook.WithHandlerTimeout
	WithRequireHTTP2     = basewebhook.WithRequireHTTP2
	WithDedupTTL         = basewebhook.WithDedupTTL
	WithDedupTimeout     = basewebhook.WithDedupTimeout
	WithMaxBodyBytes     = basewebhook.WithMaxBodyBytes
	WithAutoWorkerCount  = basewebhook.WithAutoWorkerCount
	ResolveThreadID      = basewebhook.ResolveThreadID
	DedupKey             = basewebhook.DedupKey
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
