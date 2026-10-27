package transport

import "github.com/park285/iris-client-go/v3/internal/client/common"

type (
	ReplyAcceptedResponse = common.ReplyAcceptedResponse
	ReplyStatusSnapshot   = common.ReplyStatusSnapshot
	RoomListResponse      = common.RoomListResponse
	RoomSummary           = common.RoomSummary
	MemberListResponse    = common.MemberListResponse
	MemberInfo            = common.MemberInfo
	StatsResponse         = common.StatsResponse
	PeriodRange           = common.PeriodRange
	MemberStats           = common.MemberStats
	ReplyRequest          = common.ReplyRequest
	ReplyMention          = common.ReplyMention
	ReplyMentionUserID    = common.ReplyMentionUserID
	MediaChunkRequest     = common.MediaChunkRequest
	MediaChunkResponse    = common.MediaChunkResponse
)

func normalizeReplyMentionUserID(value ReplyMentionUserID) (ReplyMentionUserID, error) {
	return common.NormalizeReplyMentionUserID(value)
}
