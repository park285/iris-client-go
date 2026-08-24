package transport

import "github.com/park285/iris-client-go/v2/internal/client/query"

type (
	QueryRoomSummaryRequest    = query.QueryRoomSummaryRequest
	QueryMemberStatsRequest    = query.QueryMemberStatsRequest
	QueryRecentThreadsRequest  = query.QueryRecentThreadsRequest
	QueryRecentMessagesRequest = query.QueryRecentMessagesRequest
	ThreadListResponse         = query.ThreadListResponse
	ThreadSummary              = query.ThreadSummary
	RecentMessagesResponse     = query.RecentMessagesResponse
	RecentMessage              = query.RecentMessage
	RoomEventRecord            = query.RoomEventRecord
)
