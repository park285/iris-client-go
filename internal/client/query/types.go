package query

// QueryMemberStatsRequest는 /query/member-stats 요청입니다.
type QueryMemberStatsRequest struct {
	ChatID      int64   `json:"chatId"`
	Period      *string `json:"period,omitempty"`
	Limit       int     `json:"limit,omitempty,omitzero"`
	MinMessages int     `json:"minMessages,omitempty,omitzero"`
}

// QueryRecentMessagesRequest는 /query/recent-messages 요청입니다.
type QueryRecentMessagesRequest struct {
	ChatID         int64    `json:"chatId"`
	Limit          int      `json:"limit,omitempty,omitzero"`
	AfterID        *int64   `json:"afterId,omitempty"`
	BeforeID       *int64   `json:"beforeId,omitempty"`
	SinceCreatedAt *int64   `json:"sinceCreatedAt,omitempty"`
	UntilCreatedAt *int64   `json:"untilCreatedAt,omitempty"`
	ThreadID       *string  `json:"threadId,omitempty"`
	ChatLogIDs     []string `json:"chatLogIds,omitempty"`
}

// RecentMessagesResponse는 최근 메시지 목록 응답입니다.
type RecentMessagesResponse struct {
	ChatID   int64           `json:"chatId"`
	Messages []RecentMessage `json:"messages"`
}

// RecentMessage는 개별 메시지입니다.
type RecentMessage struct {
	SequenceID int64   `json:"sequenceId"`
	ChatLogID  string  `json:"chatLogId,omitempty"`
	ChatID     int64   `json:"chatId"`
	UserID     int64   `json:"userId"`
	Message    string  `json:"message"`
	Type       int     `json:"type"`
	CreatedAt  int64   `json:"createdAt"`
	ThreadID   *string `json:"threadId,omitempty"`
}

// RoomEventRecord는 채팅방 이벤트 기록입니다.
type RoomEventRecord struct {
	ID          int64  `json:"id"`
	ChatID      int64  `json:"chatId"`
	EventType   string `json:"eventType"`
	UserID      int64  `json:"userId"`
	Payload     string `json:"payload"`
	CreatedAtMs int64  `json:"createdAtMs"`
}

// EventTypeMemberNicknameUpdated는 Iris가 발행하는 유일한 semantic event 타입입니다.
// 일반 메시지 이벤트의 eventType은 Kakao 메시지 타입 코드 문자열("1", "2", ...)입니다.
const EventTypeMemberNicknameUpdated = "member_nickname_updated"

// MemberNicknameUpdatedEvent는 member_nickname_updated 방 이벤트의 payload입니다.
type MemberNicknameUpdatedEvent struct {
	Type                string  `json:"type"`
	SourceLogID         int64   `json:"sourceLogId"`
	RawSourceLogID      *int64  `json:"rawSourceLogId,omitempty"`
	SourceGenerationID  *int64  `json:"sourceGenerationId,omitempty"`
	SourceAccountID     string  `json:"sourceAccountId,omitempty"`
	ChatLogID           *string `json:"chatLogId,omitempty"`
	ChatID              int64   `json:"chatId"`
	UserID              int64   `json:"userId"`
	PreviousDisplayName string  `json:"previousDisplayName"`
	CurrentDisplayName  string  `json:"currentDisplayName"`
	CreatedAtMs         int64   `json:"createdAtMs"`
}
