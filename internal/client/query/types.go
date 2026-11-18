package query

type QueryMemberStatsRequest struct {
	ChatID      int64   `json:"chatId"`
	Period      *string `json:"period,omitempty"`
	Limit       int     `json:"limit,omitempty,omitzero"`
	MinMessages int     `json:"minMessages,omitempty,omitzero"`
}

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

type RecentMessagesResponse struct {
	ChatID   int64           `json:"chatId"`
	Messages []RecentMessage `json:"messages"`
}

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

type RoomEventRecord struct {
	ID          int64  `json:"id"`
	ChatID      int64  `json:"chatId"`
	EventType   string `json:"eventType"`
	UserID      int64  `json:"userId"`
	Payload     string `json:"payload"`
	CreatedAtMs int64  `json:"createdAtMs"`
}

const EventTypeMemberNicknameUpdated = "member_nickname_updated"

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
