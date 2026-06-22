package iris

import (
	"context"
	"net"

	"github.com/park285/iris-client-go/internal/client"
)

type H2CClient = client.H2CClient
type SecretRole = client.SecretRole

type Sender = client.Sender
type AdminClient = client.AdminClient
type CertReloadClient = client.CertReloadClient
type RoomClient = client.RoomClient
type RoomEventsByTypeClient = client.RoomEventsByTypeClient
type RoomUserEventsByTypeClient = client.RoomUserEventsByTypeClient
type LatestRoomUserEventsByTypeClient = client.LatestRoomUserEventsByTypeClient
type NicknameHistorySearchClient = client.NicknameHistorySearchClient
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
type CertReloadResponse = client.CertReloadResponse
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
type NicknameHistorySearchResponse = client.NicknameHistorySearchResponse
type NicknameHistorySearchMatch = client.NicknameHistorySearchMatch
type NicknameHistoryEntry = client.NicknameHistoryEntry
type KaringTemplateArgs = client.KaringTemplateArgs
type KaringStreamStatus = client.KaringStreamStatus
type KaringContentItem = client.KaringContentItem
type KaringContentListRequest = client.KaringContentListRequest
type KaringHololiveStream = client.KaringHololiveStream
type KaringSendRequest = client.KaringSendRequest
type KaringHololiveRequest = client.KaringHololiveRequest
type KaringDryRunResponse = client.KaringDryRunResponse
type MemberNicknameUpdatedEvent = client.MemberNicknameUpdatedEvent
type RawSSEEvent = client.RawSSEEvent
type SSERoomEventBody = client.SSERoomEventBody
type SSEStreamState = client.SSEStreamState
type ClientSDKConfig = client.SDKConfig

type RebindingClient = client.RebindingClient
type RebindingClientConfig = client.RebindingClientConfig

const (
	EventTypeMemberNicknameUpdated = client.EventTypeMemberNicknameUpdated

	SSEEventRoomEvent   = client.SSEEventRoomEvent
	SSEEventStreamState = client.SSEEventStreamState

	StreamCursorStatusCurrent = client.StreamCursorStatusCurrent
	StreamCursorStatusStale   = client.StreamCursorStatusStale
	StreamCursorStatusFuture  = client.StreamCursorStatusFuture

	StreamRecoveryQueryRecentMessages = client.StreamRecoveryQueryRecentMessages
)

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

	SecretRoleInbound    = client.SecretRoleInbound
	SecretRoleBotControl = client.SecretRoleBotControl
	SecretRoleCertReload = client.SecretRoleCertReload

	PathDiagnosticsChatroom     = client.PathDiagnosticsChatroom
	PathDiagnosticsNativeCore   = client.PathDiagnosticsNativeCore
	PathDiagnosticsRuntime      = client.PathDiagnosticsRuntime
	PathDiagnosticsTextPing     = client.PathDiagnosticsTextPing
	PathDiagnosticsChatroomOpen = client.PathDiagnosticsChatroomOpen
	PathAdminCertReload         = client.PathAdminCertReload

	HeaderIrisTimestamp  = client.HeaderIrisTimestamp
	HeaderIrisNonce      = client.HeaderIrisNonce
	HeaderIrisSignature  = client.HeaderIrisSignature
	HeaderIrisBodySHA256 = client.HeaderIrisBodySHA256

	PingStrategyAuto   = client.PingStrategyAuto
	PingStrategyReady  = client.PingStrategyReady
	PingStrategyHealth = client.PingStrategyHealth

	KaringStreamStatusLive     = client.KaringStreamStatusLive
	KaringStreamStatusUpcoming = client.KaringStreamStatusUpcoming
)

var (
	ResolveClientSDKConfig = client.ResolveSDKConfig

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
	WithH3CACertReloadInterval       = client.WithH3CACertReloadInterval
	WithH3InsecureSkipVerifyForTests = client.WithH3InsecureSkipVerifyForTests
	WithReplyRetry                   = client.WithReplyRetry
	WithHMACSecret                   = client.WithHMACSecret
	WithBaseURL                      = client.WithBaseURL
	WithBotToken                     = client.WithBotToken
	WithClientRequestID              = client.WithClientRequestID
	WithThreadID                     = client.WithThreadID
	WithThreadScope                  = client.WithThreadScope
	WithImageContentType             = client.WithImageContentType
	WithMention                      = client.WithMention
	WithMentions                     = client.WithMentions
	WithAttachmentJSON               = client.WithAttachmentJSON
	WithInboundSecret                = client.WithInboundSecret
	WithBotControlToken              = client.WithBotControlToken
	WithCertReloadToken              = client.WithCertReloadToken
	WithH3AllowSystemRoots           = client.WithH3AllowSystemRoots
)

func WithH3DialGuard(guard func(net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

// Client는 봇 코드가 공통으로 의존할 Iris 상위 인터페이스입니다.
type Client interface {
	Sender
	AdminClient
}

// ClosableClient는 장기 실행 consumer가 Iris transport lifecycle을 명시적으로 닫을 때 사용합니다.
type ClosableClient interface {
	Client
	Close() error
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

// ClosableFullClient는 모든 Iris 기능과 transport lifecycle을 함께 요구하는 경계 인터페이스입니다.
type ClosableFullClient interface {
	FullClient
	Close() error
}

// BotClient는 메시지 전송·liveness·config 조회만 필요한 봇 소비자용 최소 인터페이스입니다.
type BotClient interface {
	Sender
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
}

func NewH2CClient(baseURL, botToken string, opts ...ClientOption) *H2CClient {
	return client.NewH2CClient(baseURL, botToken, opts...)
}

func NewRebindingClient(cfg RebindingClientConfig) *RebindingClient {
	return client.NewRebindingClient(cfg)
}
