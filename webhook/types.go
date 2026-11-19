package webhook

import "encoding/json/jsontext"

type WebhookRequest struct {
	Route              string           `json:"route,omitempty"`
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

type Message struct {
	Msg    string
	Room   string
	Sender *string
	JSON   *MessageJSON
}

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

type WebhookMention struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty,omitzero"`
}
