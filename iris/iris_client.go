package iris

import (
	"context"
	"net"

	"github.com/park285/iris-client-go/v3/internal/client/rebind"
	client "github.com/park285/iris-client-go/v3/internal/client/transport"
)

// iris package는 v3 소비자가 사용하는 canonical public facade다. 아래 alias의 구현 소유자는
// internal/client/transport이며, rebind 같은 내부 package는 이 facade를 재노출하지 않는다.
type APIClient = client.APIClient

type Sender = client.Sender

type (
	ClientOption      = client.ClientOption
	H3DialGuardOption = client.H3DialGuardOption
	SendOption        = client.SendOption
	TransportMetrics  = client.TransportMetrics
)

type (
	ReplyRequest          = client.ReplyRequest
	ReplyMention          = client.ReplyMention
	ReplyAcceptedResponse = client.ReplyAcceptedResponse
	ReplyStatusSnapshot   = client.ReplyStatusSnapshot
	RoomListResponse      = client.RoomListResponse
	RoomSummary           = client.RoomSummary
	MemberListResponse    = client.MemberListResponse
	MemberInfo            = client.MemberInfo
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
	PathReply       = client.PathReply
	PathReplyStatus = client.PathReplyStatus
	PathReady       = client.PathReady
	PathHealth      = client.PathHealth

	HeaderIrisTimestamp  = client.HeaderIrisTimestamp
	HeaderIrisNonce      = client.HeaderIrisNonce
	HeaderIrisSignature  = client.HeaderIrisSignature
	HeaderIrisBodySHA256 = client.HeaderIrisBodySHA256
)

var (
	ResolveClientSDKConfig = client.ResolveSDKConfig

	// WithTransport는 h3 또는 http1을 고른다. 그 밖의 값은 거절한다.
	WithTransport             = client.WithTransport
	WithTimeout               = client.WithTimeout
	WithDialTimeout           = client.WithDialTimeout
	WithResponseHeaderTimeout = client.WithResponseHeaderTimeout
	WithIdleConnTimeout       = client.WithIdleConnTimeout
	WithMaxIdleConns          = client.WithMaxIdleConns
	WithMaxIdleConnsPerHost   = client.WithMaxIdleConnsPerHost
	WithLogger                = client.WithLogger
	// WithHTTPClient로 넣는 client의 transport는 요청 context 취소로 응답 본문 Read가 풀려야 한다.
	// 응답 trailer drain의 시간 상한이 그 취소에 기대기 때문이다.
	WithHTTPClient       = client.WithHTTPClient
	WithTransportMetrics = client.WithTransportMetrics
	WithH3ServerName     = client.WithH3ServerName
	WithH3CACertFile     = client.WithH3CACertFile
	WithReplyRetry       = client.WithReplyRetry
	WithBaseURL          = client.WithBaseURL
	// WithBotToken은 NewClient가 읽는 bot-control 자격의 정본 이름이다(IRIS_BOT_TOKEN과 같다).
	// NewAPIClient는 이 옵션을 읽지 않고 botToken 인자를 쓴다.
	WithBotToken                  = client.WithBotToken
	WithClientRequestID           = client.WithClientRequestID
	WithThreadID                  = client.WithThreadID
	WithImageContentType          = client.WithImageContentType
	WithMention                   = client.WithMention
	NewH3DialGuardForBaseURL      = client.NewH3DialGuardForBaseURL
	WithH3DialGuardForBaseURL     = client.WithH3DialGuardForBaseURL
	WithH3DialGuardTTL            = client.WithH3DialGuardTTL
	WithH3DialGuardResolveTimeout = client.WithH3DialGuardResolveTimeout
	WithH3DialGuardLenientInit    = client.WithH3DialGuardLenientInit
	WithH3DialGuardLogger         = client.WithH3DialGuardLogger
)

// WithThreadScope는 reply의 threadScope를 지정합니다. Iris는 threadId가 있으면 항상 thread-only scope 2로 보냅니다.
// 생략하거나 WithThreadID와 함께 2를 전달하십시오. WithThreadID 없이 사용하면 요청 전에 오류를 반환합니다.
var WithThreadScope = client.WithThreadScope

func ValidateClientRequestID(id string) error {
	return client.ValidateClientRequestID(id)
}

// WithH3DialGuard는 H3 연결 대상 IP를 dial context와 함께 검사하는 guard를 설정합니다.
func WithH3DialGuard(guard func(context.Context, net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

// BotClient는 메시지 전송·liveness만 필요한 봇 소비자용 최소 인터페이스입니다.
type BotClient interface {
	Sender
	Ping(ctx context.Context) bool
}

// NewAPIClient는 baseURL과 bot-control 서명 비밀키 botToken으로 client를 만듭니다.
func NewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	return client.NewAPIClient(baseURL, botToken, opts...)
}

func NewRebindingClient(cfg RebindingClientConfig) *RebindingClient {
	return rebind.NewRebindingClient(cfg)
}
