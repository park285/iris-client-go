package webhook

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"
)

type WebhookRequest struct {
	Route string `json:"route,omitempty"`
	// MessageID는 필수이며 X-Iris-Message-Id와 일치해야 한다.
	MessageID          string            `json:"messageId"`
	SourceLogID        int64             `json:"sourceLogId,omitempty,omitzero"`
	SourceCreatedAtMS  int64             `json:"sourceCreatedAtMs,omitempty,omitzero"`
	RawSourceLogID     *int64            `json:"rawSourceLogId,omitempty"`
	SourceGenerationID *int64            `json:"sourceGenerationId,omitempty"`
	SourceAccountID    string            `json:"sourceAccountId,omitempty"`
	Text               string            `json:"text"`
	Room               string            `json:"room"`
	Sender             string            `json:"sender"`
	UserID             string            `json:"userId"`
	ChatLogID          string            `json:"chatLogId,omitempty"`
	RoomType           string            `json:"roomType,omitempty"`
	RoomLinkID         string            `json:"roomLinkId,omitempty"`
	ThreadID           string            `json:"threadId,omitempty"`
	ThreadScope        *int              `json:"threadScope,omitempty"`
	Type               string            `json:"type,omitempty"`
	IsMine             *bool             `json:"isMine,omitempty"`
	Origin             string            `json:"origin,omitempty"`
	Attachment         string            `json:"attachment,omitempty"`
	Mentions           []WebhookMention  `json:"mentions,omitempty"`
	EventPayload       jsonv1.RawMessage `json:"eventPayload,omitempty"`
}

// Message는 Handler가 전달하는 메시지다. 본문과 방의 정본은 Msg와 Room이다.
type Message struct {
	Msg    string       `json:"msg"`
	Room   string       `json:"room"`
	Sender *string      `json:"sender,omitempty"`
	JSON   *MessageJSON `json:"json,omitempty"`
}

type MessageJSON struct {
	UserID             string            `json:"user_id,omitempty"`
	Type               string            `json:"type,omitempty"`
	Route              string            `json:"route,omitempty"`
	MessageID          string            `json:"message_id,omitempty"`
	ChatLogID          string            `json:"chat_log_id,omitempty"`
	RoomType           string            `json:"room_type,omitempty"`
	RoomLinkID         string            `json:"room_link_id,omitempty"`
	SourceLogID        *int64            `json:"source_log_id,omitempty"`
	SourceCreatedAtMS  int64             `json:"source_created_at_ms,omitempty,omitzero"`
	RawSourceLogID     *int64            `json:"raw_source_log_id,omitempty"`
	SourceGenerationID *int64            `json:"source_generation_id,omitempty"`
	SourceAccountID    string            `json:"source_account_id,omitempty"`
	ThreadID           *string           `json:"thread_id,omitempty"`
	ThreadScope        *int              `json:"thread_scope,omitempty"`
	IsMine             *bool             `json:"is_mine,omitempty"`
	Origin             string            `json:"origin,omitempty"`
	Attachment         string            `json:"attachment,omitempty"`
	Mentions           []WebhookMention  `json:"mentions,omitempty"`
	EventPayload       jsonv1.RawMessage `json:"event_payload,omitempty"`
}

// UnmarshalJSON은 제거된 중복 본문·방 필드를 거절한다.
func (m *MessageJSON) UnmarshalJSON(data []byte) error {
	var fields map[string]jsontext.Value

	if err := jsonv2.Unmarshal(data, &fields); err != nil {
		return err
	}

	for _, name := range []string{"message", "chat_id"} {
		if _, legacy := fields[name]; legacy {
			return errors.New("iris webhook: retired MessageJSON field " + name)
		}
	}

	type canonical MessageJSON

	return jsonv2.Unmarshal(data, (*canonical)(m))
}

// WebhookMention은 webhook payload의 멘션 대상 하나다. 정본 wire 형식은 문자열 userId이며
// Iris는 이 형식만 보낸다.
type WebhookMention struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty,omitzero"`
}

// UnmarshalJSON은 정본 문자열 userId만 수용한다.
func (m *WebhookMention) UnmarshalJSON(data []byte) error {
	var fields map[string]jsontext.Value

	if err := jsonv2.Unmarshal(data, &fields); err != nil {
		return err
	}

	if _, legacy := fields["user_id"]; legacy {
		return errors.New("iris webhook: retired mention user_id")
	}

	type canonical WebhookMention

	if err := jsonv2.Unmarshal(data, (*canonical)(m)); err != nil {
		return err
	}

	m.UserID = strings.TrimSpace(m.UserID)
	if m.UserID == "" {
		return errors.New("iris webhook: mention userId is required")
	}

	m.Nickname = strings.TrimSpace(m.Nickname)

	return nil
}
