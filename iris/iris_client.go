package iris

import (
	"context"
	"encoding/json"
	"net"

	"github.com/park285/iris-client-go/internal/client/rebind"
	client "github.com/park285/iris-client-go/internal/client/transport"
)

type H2CClient = client.H2CClient

type Sender = client.Sender
type KaringClient = client.KaringClient

type ClientOption = client.ClientOption
type H3DialGuardOption = client.H3DialGuardOption
type SendOption = client.SendOption
type TransportMetrics = client.TransportMetrics
type NoopTransportMetrics = client.NoopTransportMetrics

type ReplyRequest = client.ReplyRequest
type ReplyMention = client.ReplyMention
type ConfigResponse = client.ConfigResponse
type ConfigUpdateRequest = client.ConfigUpdateRequest
type ConfigUpdateResponse = client.ConfigUpdateResponse
type CertReloadResponse = client.CertReloadResponse
type ReplyAcceptedResponse = client.ReplyAcceptedResponse
type ReplyStatusSnapshot = client.ReplyStatusSnapshot
type BridgeHealthResult = client.BridgeHealthResult
type NativeCoreDiagnostics = client.NativeCoreDiagnostics
type TextPingWarmResponse = client.TextPingWarmResponse
type RoomListResponse = client.RoomListResponse
type RoomSummary = client.RoomSummary
type MemberListResponse = client.MemberListResponse
type MemberInfo = client.MemberInfo
type StatsResponse = client.StatsResponse
type MemberStats = client.MemberStats
type QueryMemberStatsRequest = client.QueryMemberStatsRequest
type QueryRecentMessagesRequest = client.QueryRecentMessagesRequest
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
type KaringSendRequest = client.KaringSendRequest
type KaringHololiveRequest = client.KaringHololiveRequest
type KaringDryRunResponse = client.KaringDryRunResponse
type MemberNicknameUpdatedEvent = client.MemberNicknameUpdatedEvent
type ClientSDKConfig = client.SDKConfig

type RebindingClient = rebind.RebindingClient
type RebindingClientConfig = rebind.RebindingClientConfig

const (
	EventTypeMemberNicknameUpdated = client.EventTypeMemberNicknameUpdated
)

const (
	PathReply             = client.PathReply
	PathReplyStatus       = client.PathReplyStatus
	PathReady             = client.PathReady
	PathHealth            = client.PathHealth
	PathKaringSend        = client.PathKaringSend
	PathKaringContentList = client.PathKaringContentList
	PathKaringHololive    = client.PathKaringHololive

	HeaderIrisTimestamp  = client.HeaderIrisTimestamp
	HeaderIrisNonce      = client.HeaderIrisNonce
	HeaderIrisSignature  = client.HeaderIrisSignature
	HeaderIrisBodySHA256 = client.HeaderIrisBodySHA256

	KaringStreamStatusLive     = client.KaringStreamStatusLive
	KaringStreamStatusUpcoming = client.KaringStreamStatusUpcoming
)

var (
	ResolveClientSDKConfig = client.ResolveSDKConfig

	WithTransport                 = client.WithTransport
	WithTimeout                   = client.WithTimeout
	WithDialTimeout               = client.WithDialTimeout
	WithResponseHeaderTimeout     = client.WithResponseHeaderTimeout
	WithIdleConnTimeout           = client.WithIdleConnTimeout
	WithMaxIdleConns              = client.WithMaxIdleConns
	WithMaxIdleConnsPerHost       = client.WithMaxIdleConnsPerHost
	WithLogger                    = client.WithLogger
	WithHTTPClient                = client.WithHTTPClient
	WithTransportMetrics          = client.WithTransportMetrics
	WithH3ServerName              = client.WithH3ServerName
	WithH3CACertFile              = client.WithH3CACertFile
	WithReplyRetry                = client.WithReplyRetry
	WithHMACSecret                = client.WithHMACSecret
	WithBaseURL                   = client.WithBaseURL
	WithBotToken                  = client.WithBotToken
	WithClientRequestID           = client.WithClientRequestID
	WithThreadID                  = client.WithThreadID
	WithThreadScope               = client.WithThreadScope
	WithImageContentType          = client.WithImageContentType
	WithMention                   = client.WithMention
	WithMentions                  = client.WithMentions
	WithInboundSecret             = client.WithInboundSecret
	WithBotControlToken           = client.WithBotControlToken
	WithH3AllowSystemRoots        = client.WithH3AllowSystemRoots
	NewH3DialGuardForBaseURL      = client.NewH3DialGuardForBaseURL
	WithH3DialGuardForBaseURL     = client.WithH3DialGuardForBaseURL
	WithH3DialGuardTTL            = client.WithH3DialGuardTTL
	WithH3DialGuardResolveTimeout = client.WithH3DialGuardResolveTimeout
	WithH3DialGuardLenientInit    = client.WithH3DialGuardLenientInit
	WithH3DialGuardLogger         = client.WithH3DialGuardLogger
)

func WithH3DialGuard(guard func(net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

func WithH3DialGuardContext(guard func(context.Context, net.IP) error) ClientOption {
	return client.WithH3DialGuardContext(guard)
}

// Client는 봇 코드가 공통으로 의존할 Iris 상위 인터페이스입니다.
type Client interface {
	Sender
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
	UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error)
	GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error)
	GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error)
	GetRuntimeDiagnostics(ctx context.Context) (json.RawMessage, error)
	GetChatroomFields(ctx context.Context, chatID int64) (json.RawMessage, error)
	OpenChatroom(ctx context.Context, chatID int64) (json.RawMessage, error)
	GetTextPingDiagnostics(ctx context.Context, chatID int64) (json.RawMessage, error)
	WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error)
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
	return rebind.NewRebindingClient(cfg)
}
