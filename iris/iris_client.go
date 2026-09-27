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
	// NativeCoreDiagnostics는 GetNativeCoreDiagnostics의 응답이다.
	//
	// Deprecated: Iris가 /diagnostics/native-core route를 삭제했다. 같은 값은 GetRuntimeDiagnostics
	// 응답의 nativeCore 객체에 있다. 다음 coordinated major에서 삭제한다.
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
	// KaringHololiveRequest는 SendKaringHololive의 요청이다.
	//
	// Deprecated: Iris가 /karing/hololive route와 stream·streams 입력을 삭제했다.
	// SendKaringContentList와 KaringContentListRequest.Items를 쓴다. 다음 coordinated major에서 삭제한다.
	KaringHololiveRequest      = client.KaringHololiveRequest
	KaringDryRunResponse       = client.KaringDryRunResponse
	MemberNicknameUpdatedEvent = client.MemberNicknameUpdatedEvent
	ClientSDKConfig            = client.SDKConfig
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
	// PathKaringHololive는 Iris가 삭제한 /karing/content-list alias route다.
	//
	// Deprecated: PathKaringContentList를 쓴다. 다음 coordinated major에서 삭제한다.
	PathKaringHololive = client.PathKaringHololive

	HeaderIrisTimestamp  = client.HeaderIrisTimestamp
	HeaderIrisNonce      = client.HeaderIrisNonce
	HeaderIrisSignature  = client.HeaderIrisSignature
	HeaderIrisBodySHA256 = client.HeaderIrisBodySHA256

	KaringStreamStatusLive     = client.KaringStreamStatusLive
	KaringStreamStatusUpcoming = client.KaringStreamStatusUpcoming
)

var (
	ResolveClientSDKConfig = client.ResolveSDKConfig

	// WithTransport는 transport를 고른다. 정본 값은 h3와 http1이다. 별칭 http3·http/3·quic과 http·http/1.1은
	// 폐기 예정 입력이며(IRIS_TRANSPORT 환경값 포함) 다음 coordinated major에서 거절한다
	// (DEC-20260926-stack-iris-client-go-compat-surface-retirement).
	WithTransport             = client.WithTransport
	WithTimeout               = client.WithTimeout
	WithDialTimeout           = client.WithDialTimeout
	WithResponseHeaderTimeout = client.WithResponseHeaderTimeout
	WithIdleConnTimeout       = client.WithIdleConnTimeout
	WithMaxIdleConns          = client.WithMaxIdleConns
	WithMaxIdleConnsPerHost   = client.WithMaxIdleConnsPerHost
	WithLogger                = client.WithLogger
	WithHTTPClient            = client.WithHTTPClient
	WithTransportMetrics      = client.WithTransportMetrics
	WithH3ServerName          = client.WithH3ServerName
	WithH3CACertFile          = client.WithH3CACertFile
	WithReplyRetry            = client.WithReplyRetry
	// WithHMACSecret는 역할 사이 공유 서명 비밀키다. WithInboundSecret이 없으면 /config*에,
	// WithBotControlToken이 없으면 bot token 대신 bot-control 라우트에 쓰인다.
	//
	// Deprecated: Iris 서버는 Inbound와 BotControl 두 역할만 두고 역할 사이 폴백이 없다. /config*
	// 서명 값은 WithInboundSecret으로, bot-control 서명 값은 NewAPIClient의 botToken 인자(NewClient는
	// WithBotToken 또는 IRIS_BOT_TOKEN)로 옮긴다. 다음 coordinated major에서 공유 비밀 폴백과 함께
	// 삭제한다(DEC-20260926-stack-iris-client-go-role-secrets).
	WithHMACSecret = client.WithHMACSecret
	WithBaseURL    = client.WithBaseURL
	// WithBotToken은 NewClient가 읽는 bot-control 자격의 정본 이름이다(IRIS_BOT_TOKEN과 같다).
	// NewAPIClient는 이 옵션을 읽지 않고 botToken 인자를 쓴다.
	WithBotToken         = client.WithBotToken
	WithClientRequestID  = client.WithClientRequestID
	WithThreadID         = client.WithThreadID
	WithImageContentType = client.WithImageContentType
	WithMention          = client.WithMention
	WithMentions         = client.WithMentions
	WithInboundSecret    = client.WithInboundSecret
	// WithBotControlToken은 bot-control 라우트 서명 비밀키를 bot token과 다른 값으로 지정한다.
	//
	// Deprecated: 같은 bot-control 자격의 정본 이름은 NewAPIClient의 botToken 인자(NewClient는
	// WithBotToken 또는 IRIS_BOT_TOKEN)다. 이 옵션에 넘기던 값을 그 자리로 옮긴다. 다음 coordinated
	// major에서 삭제한다(DEC-20260926-stack-iris-client-go-role-secrets).
	WithBotControlToken = client.WithBotControlToken
	// WithCertReloadToken은 ReloadH3Certificate 서명 비밀키를 지정한다. SDK v2의 ReloadH3Certificate는
	// 이 값이 없으면 ErrCertReloadTokenRequired를 반환하므로, 호출하는 소비자는 bot-control 자격과
	// 같은 값을 넘긴다.
	//
	// Deprecated: Iris에는 cert-reload 역할이 없고 /admin/cert-reload를 bot-control 자격으로 검증한다.
	// 다음 coordinated major에서 이 옵션을 삭제하고 ReloadH3Certificate는 bot-control 자격으로
	// 서명한다(DEC-20260926-stack-iris-client-go-role-secrets). 그때 이 옵션 호출만 지우면 된다.
	WithCertReloadToken           = client.WithCertReloadToken
	WithH3AllowSystemRoots        = client.WithH3AllowSystemRoots
	NewH3DialGuardForBaseURL      = client.NewH3DialGuardForBaseURL
	WithH3DialGuardForBaseURL     = client.WithH3DialGuardForBaseURL
	WithH3DialGuardTTL            = client.WithH3DialGuardTTL
	WithH3DialGuardResolveTimeout = client.WithH3DialGuardResolveTimeout
	WithH3DialGuardLenientInit    = client.WithH3DialGuardLenientInit
	WithH3DialGuardLogger         = client.WithH3DialGuardLogger
)

// WithThreadScope는 reply 요청의 threadScope 값을 지정합니다. Iris는 threadId가 있는 reply를 이
// 값과 관계없이 thread-only scope 2로 보냅니다. 요청 값으로 전달 범위를 조정하지 않는 것이 의도한
// 계약입니다(DEC-20260926-iris-reply-thread-scope-fixed). 생략하거나 WithThreadID와 함께 2를
// 전달하며, WithThreadID 없이 쓰면 값과 관계없이 요청 전에 오류를 반환합니다.
var WithThreadScope = client.WithThreadScope

func ValidateClientRequestID(id string) error {
	return client.ValidateClientRequestID(id)
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
	// Deprecated: Iris가 /diagnostics/native-core route를 삭제했다. GetRuntimeDiagnostics 응답의
	// nativeCore 객체를 읽는다. 다음 coordinated major에서 이 메서드를 Client에서 삭제한다.
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

// NewAPIClient는 baseURL과 bot-control 자격 botToken으로 client를 만든다. 인자 botToken은 Iris
// BotControl 역할(/reply, /rooms, /events, /karing, /diagnostics 등)의 정본 서명 비밀키이고,
// /config* 서명의 정본은 WithInboundSecret이다(DEC-20260926-stack-iris-client-go-role-secrets).
func NewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	return client.NewAPIClient(baseURL, botToken, opts...)
}

func NewRebindingClient(cfg RebindingClientConfig) *RebindingClient {
	return rebind.NewRebindingClient(cfg)
}
