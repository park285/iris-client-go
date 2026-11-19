package iris

import (
	"context"
	"net"

	"github.com/park285/iris-client-go/v5/internal/client/common"
	"github.com/park285/iris-client-go/v5/internal/client/query"
	client "github.com/park285/iris-client-go/v5/internal/client/transport"
	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

type APIClient = client.APIClient

type Sender = client.Sender

type (
	ClientOption      = client.ClientOption
	H3DialGuardOption = client.H3DialGuardOption
	SendOption        = client.SendOption
	TransportMetrics  = client.TransportMetrics
)

type (
	ReplyMention          = common.ReplyMention
	ReplyAcceptedResponse = common.ReplyAcceptedResponse
	ReplyStatusSnapshot   = common.ReplyStatusSnapshot
	RoomListResponse      = common.RoomListResponse
	RoomSummary           = common.RoomSummary
	MemberListResponse    = common.MemberListResponse
	MemberInfo            = common.MemberInfo
)

type (
	StatsResponse                 = common.StatsResponse
	MemberStats                   = common.MemberStats
	QueryMemberStatsRequest       = query.QueryMemberStatsRequest
	QueryRecentMessagesRequest    = query.QueryRecentMessagesRequest
	RecentMessagesResponse        = query.RecentMessagesResponse
	RecentMessage                 = query.RecentMessage
	RoomEventRecord               = query.RoomEventRecord
	NicknameHistorySearchResponse = client.NicknameHistorySearchResponse
	NicknameHistorySearchMatch    = client.NicknameHistorySearchMatch
	NicknameHistoryEntry          = client.NicknameHistoryEntry
	MemberNicknameUpdatedEvent    = query.MemberNicknameUpdatedEvent
)

const (
	EventTypeMemberNicknameUpdated = query.EventTypeMemberNicknameUpdated
)

const (
	TransportH3    = client.TransportH3
	TransportHTTP1 = client.TransportHTTP1
)

const (
	PathReply       = client.PathReply
	PathReplyStatus = client.PathReplyStatus
	PathReady       = client.PathReady
	PathHealth      = client.PathHealth

	HeaderIrisTimestamp  = irishmac.HeaderIrisTimestamp
	HeaderIrisNonce      = irishmac.HeaderIrisNonce
	HeaderIrisSignature  = irishmac.HeaderIrisSignature
	HeaderIrisBodySHA256 = irishmac.HeaderIrisBodySHA256
)

var (
	ParseTransport = client.ParseTransport

	WithTransport                 = client.WithTransport
	WithTimeout                   = client.WithTimeout
	WithHTTPClient                = client.WithHTTPClient
	WithTransportMetrics          = client.WithTransportMetrics
	WithH3ServerName              = client.WithH3ServerName
	WithH3CACertFile              = client.WithH3CACertFile
	WithReplyRetry                = client.WithReplyRetry
	WithBaseURL                   = client.WithBaseURL
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

func WithH3DialGuard(guard func(context.Context, net.IP) error) ClientOption {
	return client.WithH3DialGuard(guard)
}

type BotClient interface {
	Sender
	Ping(ctx context.Context) error
}
