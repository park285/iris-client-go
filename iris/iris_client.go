package iris

import (
	"context"
	"net"

	client "github.com/park285/iris-client-go/v4/internal/client/transport"
)

// iris package는 v4 소비자가 사용하는 canonical public facade다. 아래 alias의 구현 소유자는
// internal/client/transport다.
type APIClient = client.APIClient

type Sender = client.Sender

type (
	ClientOption      = client.ClientOption
	H3DialGuardOption = client.H3DialGuardOption
	SendOption        = client.SendOption
	TransportMetrics  = client.TransportMetrics
)

type (
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
)

const (
	EventTypeMemberNicknameUpdated = client.EventTypeMemberNicknameUpdated
)

// TransportH3와 TransportHTTP1은 WithTransport·ParseTransport가 받는 정본 값이다. Http1은 loopback의
// GET /health·/ready probe 전용이다.
const (
	TransportH3    = client.TransportH3
	TransportHTTP1 = client.TransportHTTP1
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
	// ParseTransport는 transport 설정 값을 h3 또는 http1로 해석하고 빈 값과 그 밖의 값은 거절한다.
	// 소비자가 기동 시 설정을 검증할 때 SDK와 같은 어휘를 쓰게 한다.
	ParseTransport = client.ParseTransport

	// WithTransport는 h3 또는 http1을 고른다. 비우면 h3이고, 그 밖의 값은 NewClient가 거절한다.
	WithTransport = client.WithTransport
	WithTimeout   = client.WithTimeout
	// WithHTTPClient로 넣는 client의 transport는 요청 context 취소로 응답 본문 Read가 풀려야 한다.
	// 응답 trailer drain의 시간 상한이 그 취소에 기대기 때문이다.
	WithHTTPClient       = client.WithHTTPClient
	WithTransportMetrics = client.WithTransportMetrics
	WithH3ServerName     = client.WithH3ServerName
	WithH3CACertFile     = client.WithH3CACertFile
	WithReplyRetry       = client.WithReplyRetry
	WithBaseURL          = client.WithBaseURL
	// WithBotToken은 NewClient가 요구하는 bot-control 서명 자격이다.
	WithBotToken                  = client.WithBotToken
	WithClientRequestID           = client.WithClientRequestID
	WithThreadID                  = client.WithThreadID
	WithImageContentType          = client.WithImageContentType
	WithMention                   = client.WithMention
	NewH3DialGuardForBaseURL      = client.NewH3DialGuardForBaseURL
	WithH3DialGuardForBaseURL     = client.WithH3DialGuardForBaseURL
	WithH3DialGuardTTL            = client.WithH3DialGuardTTL
	WithH3DialGuardResolveTimeout = client.WithH3DialGuardResolveTimeout
	WithH3DialGuardLogger         = client.WithH3DialGuardLogger
)

func ValidateClientRequestID(id string) error {
	return client.ValidateClientRequestID(id)
}

// WithH3DialGuard는 H3 연결 대상 IP를 dial context와 함께 검사하는 guard를 설정합니다.
func WithH3DialGuard(guard func(context.Context, net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

// BotClient는 메시지 전송·readiness만 필요한 봇 소비자용 최소 인터페이스입니다. Ping은 GET /ready를
// 한 번 확인하고 실패 원인을 error로 돌려줍니다. 재시도와 로그는 호출자가 소유합니다.
type BotClient interface {
	Sender
	Ping(ctx context.Context) error
}
