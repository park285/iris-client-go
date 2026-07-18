package sse

import "encoding/json"

const (
	// EventTypeMemberNicknameUpdated는 Iris가 발행하는 유일한 semantic event 타입입니다.
	// 일반 메시지 이벤트의 eventType은 Kakao 메시지 타입 코드 문자열("1", "2", ...)입니다.
	EventTypeMemberNicknameUpdated = "member_nickname_updated"

	SSEEventRoomEvent   = "room_event"
	SSEEventStreamState = "iris.stream_state"

	StreamCursorStatusCurrent = "current"
	StreamCursorStatusStale   = "stale"
	StreamCursorStatusFuture  = "future"

	StreamRecoveryQueryRecentMessages = "query_recent_messages"
)

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

type RawSSEEvent struct {
	ID    int64
	Event string
	Data  json.RawMessage
}

// SSERoomEventBody는 SSE room_event 프레임의 data 본문입니다.
// Payload는 room_events.payload 객체가 인라인된 JSON입니다(문자열 아님).
type SSERoomEventBody struct {
	RoomEventID int64           `json:"roomEventId"`
	ChatID      int64           `json:"chatId"`
	EventType   string          `json:"eventType"`
	UserID      int64           `json:"userId"`
	Payload     json.RawMessage `json:"payload"`
}

// SSEStreamState는 SSE iris.stream_state 프레임의 data 본문입니다.
// replay 커서가 current가 아닐 때만 전송됩니다.
type SSEStreamState struct {
	CursorStatus        string `json:"cursorStatus"`
	LastEventID         int64  `json:"lastEventId"`
	OldestAvailableID   *int64 `json:"oldestAvailableId"`
	LatestAvailableID   *int64 `json:"latestAvailableId"`
	RecommendedRecovery string `json:"recommendedRecovery"`
}
