package iris

import (
	"context"
	jsonv1 "encoding/json"
	"net"

	"github.com/park285/iris-client-go/v2/internal/client/rebind"
	client "github.com/park285/iris-client-go/v2/internal/client/transport"
)

// iris package는 v2 소비자가 사용하는 canonical public facade다. 아래 alias의 구현 소유자는
// internal/client/transport이며, rebind 같은 내부 package는 이 facade를 재노출하지 않는다.
type APIClient = client.APIClient

type (
	Sender       = client.Sender
	KaringClient = client.KaringClient
)

type (
	ClientOption         = client.ClientOption
	H3DialGuardOption    = client.H3DialGuardOption
	SendOption           = client.SendOption
	TransportMetrics     = client.TransportMetrics
	NoopTransportMetrics = client.NoopTransportMetrics
)

type (
	ReplyRequest          = client.ReplyRequest
	ReplyMention          = client.ReplyMention
	ConfigResponse        = client.ConfigResponse
	ConfigUpdateRequest   = client.ConfigUpdateRequest
	ConfigUpdateResponse  = client.ConfigUpdateResponse
	CertReloadResponse    = client.CertReloadResponse
	ReplyAcceptedResponse = client.ReplyAcceptedResponse
	ReplyStatusSnapshot   = client.ReplyStatusSnapshot
	BridgeHealthResult    = client.BridgeHealthResult
	NativeCoreDiagnostics = client.NativeCoreDiagnostics
	TextPingWarmResponse  = client.TextPingWarmResponse
	RoomListResponse      = client.RoomListResponse
	RoomSummary           = client.RoomSummary
	MemberListResponse    = client.MemberListResponse
	MemberInfo            = client.MemberInfo
)

// GetRoomInfo/GetMemberActivity의 반환 타입과 그 필드 타입. 재노출하지 않으면 소비자가
// 호출은 할 수 있어도 결과를 담을 변수나 시그니처를 선언할 수 없다.
type (
	RoomInfoResponse       = client.RoomInfoResponse
	NoticeInfo             = client.NoticeInfo
	BotCommandInfo         = client.BotCommandInfo
	OpenLinkInfo           = client.OpenLinkInfo
	MemberActivityResponse = client.MemberActivityResponse
	PeriodRange            = client.PeriodRange
)

type (
	StatsResponse                 = client.StatsResponse
	MemberStats                   = client.MemberStats
	QueryMemberStatsRequest       = client.QueryMemberStatsRequest
	QueryRecentMessagesRequest    = client.QueryRecentMessagesRequest
	RecentMessagesResponse        = client.RecentMessagesResponse
	RecentMessage                 = client.RecentMessage
	RoomEventRecord               = client.RoomEventRecord
	NicknameHistorySearchResponse = client.NicknameHistorySearchResponse
	NicknameHistorySearchMatch    = client.NicknameHistorySearchMatch
	NicknameHistoryEntry          = client.NicknameHistoryEntry
	KaringTemplateArgs            = client.KaringTemplateArgs
	KaringStreamStatus            = client.KaringStreamStatus
	KaringContentItem             = client.KaringContentItem
	KaringContentListRequest      = client.KaringContentListRequest
	KaringSendRequest             = client.KaringSendRequest
	KaringHololiveRequest         = client.KaringHololiveRequest
	KaringDryRunResponse          = client.KaringDryRunResponse
	MemberNicknameUpdatedEvent    = client.MemberNicknameUpdatedEvent
	ClientSDKConfig               = client.SDKConfig
)

type (
	RebindingClient       = rebind.RebindingClient
	RebindingClientConfig = rebind.RebindingClientConfig
)

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
	WithCertReloadToken           = client.WithCertReloadToken
	WithH3AllowSystemRoots        = client.WithH3AllowSystemRoots
	NewH3DialGuardForBaseURL      = client.NewH3DialGuardForBaseURL
	WithH3DialGuardForBaseURL     = client.WithH3DialGuardForBaseURL
	WithH3DialGuardTTL            = client.WithH3DialGuardTTL
	WithH3DialGuardResolveTimeout = client.WithH3DialGuardResolveTimeout
	WithH3DialGuardLenientInit    = client.WithH3DialGuardLenientInit
	WithH3DialGuardLogger         = client.WithH3DialGuardLogger
)

func ValidateClientRequestID(id string) error {
	return client.ValidateClientRequestID(id) //nolint:wrapcheck // 공개 API는 내부 구현의 오류를 그대로 노출한다.
}

func WithH3DialGuard(guard func(net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

func WithH3DialGuardContext(guard func(context.Context, net.IP) error) ClientOption {
	return client.WithH3DialGuardContext(guard)
}

// Client는 봇 코드가 공통으로 의존할 Iris 상위 인터페이스입니다.
//
//nolint:interfacebloat // Iris 운영 API 전체를 묶는 공개 계약이며 다운스트림 호환을 위해 하나로 유지한다.
type Client interface {
	Sender
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
	UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error)
	GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error)
	GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error)
	GetRuntimeDiagnostics(ctx context.Context) (jsonv1.RawMessage, error)
	GetChatroomFields(ctx context.Context, chatID int64) (jsonv1.RawMessage, error)
	OpenChatroom(ctx context.Context, chatID int64) (jsonv1.RawMessage, error)
	GetTextPingDiagnostics(ctx context.Context, chatID int64) (jsonv1.RawMessage, error)
	WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error)
}

// BotClient는 메시지 전송·liveness·config 조회만 필요한 봇 소비자용 최소 인터페이스입니다.
type BotClient interface {
	Sender
	Ping(ctx context.Context) bool
	GetConfig(ctx context.Context) (*ConfigResponse, error)
}

func NewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	return client.NewAPIClient(baseURL, botToken, opts...)
}

func NewRebindingClient(cfg RebindingClientConfig) *RebindingClient {
	return rebind.NewRebindingClient(cfg)
}
