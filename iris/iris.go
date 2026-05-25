package iris

import (
	"context"
	"log/slog"

	"github.com/park285/iris-client-go/internal/client"
	basewebhook "github.com/park285/iris-client-go/webhook"
)

type H2CClient = client.H2CClient
type SecretRole = client.SecretRole
type HTTPError = client.HTTPError
type TransportError = client.TransportError
type PingError = client.PingError

type Sender = client.Sender
type AdminClient = client.AdminClient
type RoomClient = client.RoomClient
type EventStreamClient = client.EventStreamClient
type QueryClient = client.QueryClient
type KaringClient = client.KaringClient

type ClientOption = client.ClientOption
type SendOption = client.SendOption
type PingStrategy = client.PingStrategy
type RoomStatsOptions = client.RoomStatsOptions

type ReplyRequest = client.ReplyRequest
type ReplyMention = client.ReplyMention
type ReplyMentionUserID = client.ReplyMentionUserID
type ConfigResponse = client.ConfigResponse
type ConfigState = client.ConfigState
type ConfigDiscoveredState = client.ConfigDiscoveredState
type ConfigPendingRestart = client.ConfigPendingRestart
type ConfigUpdateRequest = client.ConfigUpdateRequest
type ConfigUpdateResponse = client.ConfigUpdateResponse
type ReplyAcceptedResponse = client.ReplyAcceptedResponse
type ReplyStatusSnapshot = client.ReplyStatusSnapshot
type BridgeHealthResult = client.BridgeHealthResult
type BridgeHealthCheck = client.BridgeHealthCheck
type BridgeDiscoveryHook = client.BridgeDiscoveryHook
type BridgeDiagnosticsCapability = client.BridgeDiagnosticsCapability
type BridgeDiagnosticsCapabilities = client.BridgeDiagnosticsCapabilities
type KeyCacheStats = client.KeyCacheStats
type NativeCoreDiagnostics = client.NativeCoreDiagnostics
type TextPingWarmResponse = client.TextPingWarmResponse
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
type QueryRoomSummaryRequest = client.QueryRoomSummaryRequest
type QueryMemberStatsRequest = client.QueryMemberStatsRequest
type QueryRecentThreadsRequest = client.QueryRecentThreadsRequest
type QueryRecentMessagesRequest = client.QueryRecentMessagesRequest
type ThreadListResponse = client.ThreadListResponse
type ThreadSummary = client.ThreadSummary
type RecentMessagesResponse = client.RecentMessagesResponse
type RecentMessage = client.RecentMessage
type RoomEventRecord = client.RoomEventRecord
type KaringTemplateArgs = client.KaringTemplateArgs
type KaringStreamStatus = client.KaringStreamStatus
type KaringContentItem = client.KaringContentItem
type KaringContentListRequest = client.KaringContentListRequest
type KaringHololiveStream = client.KaringHololiveStream
type KaringSendRequest = client.KaringSendRequest
type KaringHololiveRequest = client.KaringHololiveRequest
type KaringDryRunResponse = client.KaringDryRunResponse
type MemberEvent = client.MemberEvent
type NicknameChangeEvent = client.NicknameChangeEvent
type MemberNicknameUpdatedEvent = client.MemberNicknameUpdatedEvent
type RoleChangeEvent = client.RoleChangeEvent
type ProfileChangeEvent = client.ProfileChangeEvent
type RawSSEEvent = client.RawSSEEvent
type WebhookHandler = basewebhook.Handler
type HandlerOption = basewebhook.HandlerOption
type MessageHandler = basewebhook.MessageHandler
type Message = basewebhook.Message
type MessageJSON = basewebhook.MessageJSON
type WebhookRequest = basewebhook.WebhookRequest
type WebhookMention = basewebhook.WebhookMention
type Metrics = basewebhook.Metrics
type Deduplicator = basewebhook.Deduplicator
type TaskPool = basewebhook.TaskPool
type NoopMetrics = basewebhook.NoopMetrics
type NoopDeduplicator = basewebhook.NoopDeduplicator
type HandlerOptions = basewebhook.HandlerOptions
type WebhookReceiveDiagnostics = basewebhook.ReceiveDiagnostics
type WebhookSDKConfig = basewebhook.SDKConfig
type ClientSDKConfig = client.SDKConfig

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
	QueryClient
	EventStreamClient
	KaringClient
}

const (
	PathReply               = client.PathReply
	PathReplyStatus         = client.PathReplyStatus
	PathReady               = client.PathReady
	PathHealth              = client.PathHealth
	PathConfig              = client.PathConfig
	PathDiagnosticsBridge   = client.PathDiagnosticsBridge
	PathKaringSend          = client.PathKaringSend
	PathKaringContentList   = client.PathKaringContentList
	PathKaringHololive      = client.PathKaringHololive
	PathRooms               = client.PathRooms
	PathEventsStream        = client.PathEventsStream
	PathQueryRoomSummary    = client.PathQueryRoomSummary
	PathQueryMemberStats    = client.PathQueryMemberStats
	PathQueryRecentThreads  = client.PathQueryRecentThreads
	PathQueryRecentMessages = client.PathQueryRecentMessages
	PathWebhook             = basewebhook.PathWebhook

	SecretRoleInbound    = client.SecretRoleInbound
	SecretRoleBotControl = client.SecretRoleBotControl

	PathDiagnosticsChatroom     = client.PathDiagnosticsChatroom
	PathDiagnosticsNativeCore   = client.PathDiagnosticsNativeCore
	PathDiagnosticsRuntime      = client.PathDiagnosticsRuntime
	PathDiagnosticsTextPing     = client.PathDiagnosticsTextPing
	PathDiagnosticsChatroomOpen = client.PathDiagnosticsChatroomOpen

	HeaderIrisTimestamp  = client.HeaderIrisTimestamp
	HeaderIrisNonce      = client.HeaderIrisNonce
	HeaderIrisSignature  = client.HeaderIrisSignature
	HeaderIrisBodySHA256 = client.HeaderIrisBodySHA256
	HeaderIrisToken      = basewebhook.HeaderIrisToken
	HeaderIrisMessageID  = basewebhook.HeaderIrisMessageID

	PingStrategyAuto   = client.PingStrategyAuto
	PingStrategyReady  = client.PingStrategyReady
	PingStrategyHealth = client.PingStrategyHealth

	KaringStreamStatusLive     = client.KaringStreamStatusLive
	KaringStreamStatusUpcoming = client.KaringStreamStatusUpcoming

	DefaultDedupTTL = basewebhook.DefaultDedupTTL
)

var (
	ErrRetryable   = client.ErrRetryable
	ErrPermanent   = client.ErrPermanent
	ErrAuthFailed  = client.ErrAuthFailed
	ErrRateLimited = client.ErrRateLimited
	ErrTransport   = client.ErrTransport

	ResolveClientSDKConfig  = client.ResolveSDKConfig
	ResolveWebhookSDKConfig = basewebhook.ResolveSDKConfig

	WithTransport                    = client.WithTransport
	WithTimeout                      = client.WithTimeout
	WithDialTimeout                  = client.WithDialTimeout
	WithTLSHandshakeTimeout          = client.WithTLSHandshakeTimeout
	WithResponseHeaderTimeout        = client.WithResponseHeaderTimeout
	WithIdleConnTimeout              = client.WithIdleConnTimeout
	WithMaxIdleConns                 = client.WithMaxIdleConns
	WithMaxIdleConnsPerHost          = client.WithMaxIdleConnsPerHost
	WithMaxConnsPerHost              = client.WithMaxConnsPerHost
	WithReadIdleTimeout              = client.WithReadIdleTimeout
	WithPingTimeout                  = client.WithPingTimeout
	WithPingProbeTimeout             = client.WithPingProbeTimeout
	WithPingStrategy                 = client.WithPingStrategy
	WithWriteByteTimeout             = client.WithWriteByteTimeout
	WithLogger                       = client.WithLogger
	WithHTTPClient                   = client.WithHTTPClient
	WithRoundTripper                 = client.WithRoundTripper
	WithH3ServerName                 = client.WithH3ServerName
	WithH3CACertFile                 = client.WithH3CACertFile
	WithH3InsecureSkipVerifyForTests = client.WithH3InsecureSkipVerifyForTests
	WithReplyRetry                   = client.WithReplyRetry
	WithHMACSecret                   = client.WithHMACSecret
	WithBaseURL                      = client.WithBaseURL
	WithBotToken                     = client.WithBotToken
	WithClientRequestID              = client.WithClientRequestID
	WithThreadID                     = client.WithThreadID
	WithThreadScope                  = client.WithThreadScope
	WithMention                      = client.WithMention
	WithMentions                     = client.WithMentions
	WithAttachmentJSON               = client.WithAttachmentJSON
	WithInboundSecret                = client.WithInboundSecret
	WithBotControlToken              = client.WithBotControlToken

	WithWebhookToken    = basewebhook.WithWebhookToken
	WithWebhookLogger   = basewebhook.WithWebhookLogger
	WithContext         = basewebhook.WithContext
	WithMetrics         = basewebhook.WithMetrics
	WithDeduplicator    = basewebhook.WithDeduplicator
	WithTaskPool        = basewebhook.WithTaskPool
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

func NewH2CClient(baseURL, botToken string, opts ...ClientOption) *H2CClient {
	return client.NewH2CClient(baseURL, botToken, opts...)
}

func NewHandler(
	ctx context.Context,
	token string,
	handler MessageHandler,
	logger *slog.Logger,
	opts ...HandlerOption,
) *WebhookHandler {
	return basewebhook.NewHandler(ctx, token, handler, logger, opts...)
}
