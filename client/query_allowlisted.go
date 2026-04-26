package client

// QueryRoomSummaryRequest는 /query/room-summary 요청입니다.
type QueryRoomSummaryRequest struct {
	ChatID int64 `json:"chatId"`
}

// QueryMemberStatsRequest는 /query/member-stats 요청입니다.
type QueryMemberStatsRequest struct {
	ChatID      int64   `json:"chatId"`
	Period      *string `json:"period,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	MinMessages int     `json:"minMessages,omitempty"`
}

// QueryRecentThreadsRequest는 /query/recent-threads 요청입니다.
type QueryRecentThreadsRequest struct {
	ChatID int64 `json:"chatId"`
}

// QueryRecentMessagesRequest는 /query/recent-messages 요청입니다.
type QueryRecentMessagesRequest struct {
	ChatID   int64  `json:"chatId"`
	Limit    int    `json:"limit,omitempty"`
	AfterID  *int64 `json:"afterId,omitempty"`
	BeforeID *int64 `json:"beforeId,omitempty"`
	ThreadID *int64 `json:"threadId,omitempty"`
}

// ThreadListResponse는 스레드 목록 응답입니다.
type ThreadListResponse struct {
	ChatID  int64           `json:"chatId"`
	Threads []ThreadSummary `json:"threads"`
}

// ThreadSummary는 개별 스레드 요약입니다.
type ThreadSummary struct {
	ThreadID      string  `json:"threadId"`
	OriginMessage *string `json:"originMessage,omitempty"`
	MessageCount  int     `json:"messageCount"`
	LastActiveAt  *int64  `json:"lastActiveAt,omitempty"`
}

// RecentMessagesResponse는 최근 메시지 목록 응답입니다.
type RecentMessagesResponse struct {
	ChatID   int64           `json:"chatId"`
	Messages []RecentMessage `json:"messages"`
}

// RecentMessage는 개별 메시지입니다.
type RecentMessage struct {
	ID        int64  `json:"id"`
	ChatID    int64  `json:"chatId"`
	UserID    int64  `json:"userId"`
	Message   string `json:"message"`
	Type      int    `json:"type"`
	CreatedAt int64  `json:"createdAt"`
	ThreadID  *int64 `json:"threadId,omitempty"`
}

// RoomEventRecord는 채팅방 이벤트 기록입니다.
type RoomEventRecord struct {
	ID        int64  `json:"id"`
	ChatID    int64  `json:"chatId"`
	EventType string `json:"eventType"`
	UserID    int64  `json:"userId"`
	Payload   string `json:"payload"`
	CreatedAt int64  `json:"createdAt"`
}
