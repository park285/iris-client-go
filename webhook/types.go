package webhook

import "encoding/json/jsontext"

type WebhookRequest struct {
	Route string `json:"route,omitempty"`
	// MessageID는 필수이며 X-Iris-Message-Id와 일치해야 한다.
	MessageID          string           `json:"messageId"`
	SourceLogID        int64            `json:"sourceLogId,omitempty,omitzero"`
	SourceCreatedAtMS  int64            `json:"sourceCreatedAtMs,omitempty,omitzero"`
	RawSourceLogID     *int64           `json:"rawSourceLogId,omitempty"`
	SourceGenerationID *int64           `json:"sourceGenerationId,omitempty"`
	SourceAccountID    string           `json:"sourceAccountId,omitempty"`
	Text               string           `json:"text"`
	Room               string           `json:"room"`
	Sender             string           `json:"sender"`
	UserID             string           `json:"userId"`
	ChatLogID          string           `json:"chatLogId,omitempty"`
	RoomType           string           `json:"roomType,omitempty"`
	RoomLinkID         string           `json:"roomLinkId,omitempty"`
	ThreadID           string           `json:"threadId,omitempty"`
	ThreadScope        *int             `json:"threadScope,omitempty"`
	Type               string           `json:"type,omitempty"`
	IsMine             *bool            `json:"isMine,omitempty"`
	Origin             string           `json:"origin,omitempty"`
	Attachment         string           `json:"attachment,omitempty"`
	Mentions           []WebhookMention `json:"mentions,omitempty"`
	EventPayload       jsontext.Value   `json:"eventPayload,omitempty"`
}

// Message는 Handler가 MessageAdmitter에 전달하고 DecodeMessage가 복원하는 메시지다. 본문과 방의
// 정본은 Msg와 Room이다. 저장 형식이 아니므로 JSON 태그를 두지 않는다. Inbox에는 Iris 원본 본문을
// 저장하고 DecodeMessage로 복원한다.
type Message struct {
	Msg    string
	Room   string
	Sender *string
	JSON   *MessageJSON
}

// MessageJSON은 Message의 식별자와 이벤트 metadata다. 이름은 이전 저장 형식에서 이어졌지만 JSON
// 직렬화 계약은 없다.
type MessageJSON struct {
	UserID             string
	Type               string
	Route              string
	MessageID          string
	ChatLogID          string
	RoomType           string
	RoomLinkID         string
	SourceLogID        *int64
	SourceCreatedAtMS  int64
	RawSourceLogID     *int64
	SourceGenerationID *int64
	SourceAccountID    string
	ThreadID           *string
	ThreadScope        *int
	IsMine             *bool
	Origin             string
	Attachment         string
	Mentions           []WebhookMention
	EventPayload       jsontext.Value
}

// WebhookMention은 webhook payload의 멘션 대상 하나다. 정본 wire 형식은 문자열 userId이며
// Iris는 이 형식만 보낸다. 빈 userId는 요청 검증에서 거절한다.
type WebhookMention struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty,omitzero"`
}
