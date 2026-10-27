package transport

import "github.com/park285/iris-client-go/v4/internal/client/query"

type (
	QueryMemberStatsRequest    = query.QueryMemberStatsRequest
	QueryRecentMessagesRequest = query.QueryRecentMessagesRequest
	RecentMessagesResponse     = query.RecentMessagesResponse
	RecentMessage              = query.RecentMessage
	RoomEventRecord            = query.RoomEventRecord
	MemberNicknameUpdatedEvent = query.MemberNicknameUpdatedEvent
)

const EventTypeMemberNicknameUpdated = query.EventTypeMemberNicknameUpdated
