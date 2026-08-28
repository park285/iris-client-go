package query

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
)

// QueryRoomSummaryRequest는 /query/room-summary 요청입니다.
type QueryRoomSummaryRequest struct {
	ChatID int64 `json:"chatId"`
}

// QueryMemberStatsRequest는 /query/member-stats 요청입니다.
type QueryMemberStatsRequest struct {
	ChatID      int64   `json:"chatId"`
	Period      *string `json:"period,omitempty"`
	Limit       int     `json:"limit,omitempty,omitzero"`
	MinMessages int     `json:"minMessages,omitempty,omitzero"`
}

// QueryRecentThreadsRequest는 /query/recent-threads 요청입니다.
type QueryRecentThreadsRequest struct {
	ChatID int64 `json:"chatId"`
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
	SequenceID int64   `json:"sequenceId"`
	ChatLogID  string  `json:"chatLogId,omitempty"`
	ChatID     int64   `json:"chatId"`
	UserID     int64   `json:"userId"`
	Message    string  `json:"message"`
	Type       int     `json:"type"`
	CreatedAt  int64   `json:"createdAt"`
	ThreadID   *string `json:"threadId,omitempty"`
}

func (m *RecentMessage) UnmarshalJSON(data []byte) error {
	type recentMessageJSON struct {
		SequenceID int64          `json:"sequenceId"`
		ChatLogID  string         `json:"chatLogId,omitempty"`
		ChatID     int64          `json:"chatId"`
		UserID     int64          `json:"userId"`
		Message    string         `json:"message"`
		Type       int            `json:"type"`
		CreatedAt  int64          `json:"createdAt"`
		ThreadID   jsontext.Value `json:"threadId,omitempty"`
	}

	var raw recentMessageJSON

	if err := jsonv2.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode recent message: %w", err)
	}

	sequenceID := raw.SequenceID

	threadID, err := decodeOptionalString(raw.ThreadID)
	if err != nil {
		return fmt.Errorf("decode threadId: %w", err)
	}

	*m = RecentMessage{
		SequenceID: sequenceID,
		ChatLogID:  raw.ChatLogID,
		ChatID:     raw.ChatID,
		UserID:     raw.UserID,
		Message:    raw.Message,
		Type:       raw.Type,
		CreatedAt:  raw.CreatedAt,
		ThreadID:   threadID,
	}

	return nil
}

func decodeOptionalString(raw jsontext.Value) (*string, error) {
	if len(raw) == 0 {
		return nil, nil //nolint:nilnil // 비어 있는 원문은 값 부재를 nil 포인터로 표현한다.
	}

	decoder := jsontext.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("decode string-compatible JSON value: %w", err)
	}

	token = token.Clone()

	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("unsupported string-compatible JSON value %s", string(raw))
		}

		return nil, fmt.Errorf("decode string-compatible JSON value: %w", err)
	}

	kind := token.Kind()
	if kind == jsontext.KindNull {
		return nil, nil //nolint:nilnil // JSON null은 값 부재를 nil 포인터로 표현한다.
	}

	if kind != jsontext.KindString && kind != jsontext.KindNumber {
		return nil, fmt.Errorf("unsupported string-compatible JSON value %s", string(raw))
	}

	value := token.String()

	return &value, nil
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
