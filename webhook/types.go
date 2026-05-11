package webhook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type WebhookRequest struct {
	Route        string           `json:"route,omitempty"`
	MessageID    string           `json:"messageId,omitempty"`
	SourceLogID  int64            `json:"sourceLogId,omitempty"`
	Text         string           `json:"text"`
	Room         string           `json:"room"`
	Sender       string           `json:"sender"`
	UserID       string           `json:"userId"`
	ChatLogID    string           `json:"chatLogId,omitempty"`
	RoomType     string           `json:"roomType,omitempty"`
	RoomLinkID   string           `json:"roomLinkId,omitempty"`
	ThreadID     string           `json:"threadId,omitempty"`
	ThreadScope  *int             `json:"threadScope,omitempty"`
	Type         string           `json:"type,omitempty"`
	IsMine       *bool            `json:"isMine,omitempty"`
	Origin       string           `json:"origin,omitempty"`
	Attachment   string           `json:"attachment,omitempty"`
	Mentions     []WebhookMention `json:"mentions,omitempty"`
	EventPayload json.RawMessage  `json:"eventPayload,omitempty"`
}

type Message struct {
	Msg    string       `json:"msg"`
	Room   string       `json:"room"`
	Sender *string      `json:"sender,omitempty"`
	JSON   *MessageJSON `json:"json,omitempty"`
}

type MessageJSON struct {
	UserID       string           `json:"user_id,omitempty"`
	Message      string           `json:"message,omitempty"`
	ChatID       string           `json:"chat_id,omitempty"`
	Type         string           `json:"type,omitempty"`
	Route        string           `json:"route,omitempty"`
	MessageID    string           `json:"message_id,omitempty"`
	ChatLogID    string           `json:"chat_log_id,omitempty"`
	RoomType     string           `json:"room_type,omitempty"`
	RoomLinkID   string           `json:"room_link_id,omitempty"`
	SourceLogID  *int64           `json:"source_log_id,omitempty"`
	ThreadID     *string          `json:"thread_id,omitempty"`
	ThreadScope  *int             `json:"thread_scope,omitempty"`
	IsMine       *bool            `json:"is_mine,omitempty"`
	Origin       string           `json:"origin,omitempty"`
	Attachment   string           `json:"attachment,omitempty"`
	Mentions     []WebhookMention `json:"mentions,omitempty"`
	EventPayload json.RawMessage  `json:"event_payload,omitempty"`
}

type WebhookMention struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty"`
}

func (m *WebhookMention) UnmarshalJSON(data []byte) error {
	type webhookMentionJSON struct {
		UserID    json.RawMessage `json:"userId"`
		UserIDAlt json.RawMessage `json:"user_id"`
		Nickname  string          `json:"nickname,omitempty"`
		At        []int           `json:"at,omitempty"`
		Len       int             `json:"len,omitempty"`
	}

	var wire webhookMentionJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	userID, err := parseWebhookMentionUserID(wire.UserID)
	if err != nil {
		userID, err = parseWebhookMentionUserID(wire.UserIDAlt)
	}
	if err != nil {
		return err
	}

	m.UserID = userID
	m.Nickname = strings.TrimSpace(wire.Nickname)
	m.At = append([]int(nil), wire.At...)
	m.Len = wire.Len

	return nil
}

func parseWebhookMentionUserID(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", fmt.Errorf("iris webhook: mention userId is required")
	}

	if strings.HasPrefix(value, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			return trimmed, nil
		}

		return "", fmt.Errorf("iris webhook: mention userId must not be blank")
	}

	numeric, err := strconv.ParseInt(value, 10, 64)
	if err != nil || numeric <= 0 {
		return "", fmt.Errorf("iris webhook: mention userId must be string or positive integer")
	}

	return strconv.FormatInt(numeric, 10), nil
}
