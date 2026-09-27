package webhook

import (
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type WebhookRequest struct {
	Route string `json:"route,omitempty"`
	// MessageID는 body messageId다. Handler는 X-Iris-Message-Id header를 필수로 요구하고 body 값이 있으면
	// 둘이 같아야 받는다. 그 body 값이 비면 header 값으로 채우는 동작은 폐기 예정이며 다음 coordinated
	// major에서 body messageId를 필수로 바꾼다(DEC-20260926-stack-iris-client-go-compat-surface-retirement).
	// Iris는 두 곳에 같은 값을 항상 보내므로 Iris 발신에는 영향이 없다. 직접 서명해 보내는 테스트·smoke
	// 도구는 body messageId도 채운다.
	MessageID          string            `json:"messageId,omitempty"`
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

// Message는 Handler가 MessageHandler와 MessageAdmitter에 넘기는 메시지다. 본문과 방은 Msg·Room과
// JSON.Message·JSON.ChatID 두 곳에 같은 값으로 들어 있다. 이 이중 필드와 NewMessageContext의
// JSON 우선 폴백은 폐기 예정이며, 다음 coordinated major에서 한 필드 모델로 줄인다
// (DEC-20260926-stack-iris-client-go-compat-surface-retirement). 어느 쪽을 남길지는 그 major에서
// 정하므로, 소비자는 두 필드 사이 폴백을 직접 구현하지 말고 NewMessageContext의 Text와 RoomID로 읽는다.
type Message struct {
	Msg    string       `json:"msg"`
	Room   string       `json:"room"`
	Sender *string      `json:"sender,omitempty"`
	JSON   *MessageJSON `json:"json,omitempty"`
}

type MessageJSON struct {
	UserID             string            `json:"user_id,omitempty"`
	Message            string            `json:"message,omitempty"`
	ChatID             string            `json:"chat_id,omitempty"`
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

// WebhookMention은 webhook payload의 멘션 대상 하나다. 정본 wire 형식은 문자열 userId이며
// Iris는 이 형식만 보낸다.
type WebhookMention struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty,omitzero"`
}

// UnmarshalJSON은 정본 문자열 userId를 읽는다.
//
// 폐기 예정 호환 입력: user_id 키와 숫자 userId는 현재 생산자가 없는 호환 입력이다. 이 입력은
// v2 공개 decode 계약이라 계속 받는다. 입력을 없애 문자열 userId만 받게 하는 것은 계약
// 축소라 coordinated major에서만 할 수 있다(DEC-20260825-iris-client-go-public-surface-major-only).
// 없애기 전 조건은 ChatBotGo webhook inbox의 잔존 payload에 mentions[].user_id 키와 숫자
// userId가 0건임을 확인하는 것이다.
func (m *WebhookMention) UnmarshalJSON(data []byte) error {
	type webhookMentionJSON struct {
		UserID    jsonv1.RawMessage `json:"userId"`
		UserIDAlt jsonv1.RawMessage `json:"user_id"`
		Nickname  string            `json:"nickname,omitempty"`
		At        []int             `json:"at,omitempty"`
		Len       int               `json:"len,omitempty,omitzero"`
	}

	var wire webhookMentionJSON

	if err := jsonv2.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode webhook mention: %w", err)
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

func parseWebhookMentionUserID(raw jsonv1.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", errors.New("iris webhook: mention userId is required")
	}

	if strings.HasPrefix(value, `"`) {
		var text string

		if err := jsonv2.Unmarshal(raw, &text); err != nil {
			return "", fmt.Errorf("decode mention userId: %w", err)
		}

		if trimmed := strings.TrimSpace(text); trimmed != "" {
			return trimmed, nil
		}

		return "", errors.New("iris webhook: mention userId must not be blank")
	}

	numeric, err := strconv.ParseInt(value, 10, 64)
	if err != nil || numeric <= 0 {
		return "", errors.New("iris webhook: mention userId must be string or positive integer")
	}

	return strconv.FormatInt(numeric, 10), nil
}
