package webhook

type WebhookRequest struct {
	Route       string `json:"route,omitempty"`
	MessageID   string `json:"messageId,omitempty"`
	SourceLogID int64  `json:"sourceLogId,omitempty"`
	Text        string `json:"text"`
	Room        string `json:"room"`
	Sender      string `json:"sender"`
	UserID      string `json:"userId"`
	ChatLogID   string `json:"chatLogId,omitempty"`
	RoomType    string `json:"roomType,omitempty"`
	RoomLinkID  string `json:"roomLinkId,omitempty"`
	ThreadID    string `json:"threadId,omitempty"`
	ThreadScope *int   `json:"threadScope,omitempty"`
}

type Message struct {
	Msg    string       `json:"msg"`
	Room   string       `json:"room"`
	Sender *string      `json:"sender,omitempty"`
	JSON   *MessageJSON `json:"json,omitempty"`
}

type MessageJSON struct {
	UserID      string  `json:"user_id,omitempty"`
	Message     string  `json:"message,omitempty"`
	ChatID      string  `json:"chat_id,omitempty"`
	Type        string  `json:"type,omitempty"`
	Route       string  `json:"route,omitempty"`
	MessageID   string  `json:"message_id,omitempty"`
	ChatLogID   string  `json:"chat_log_id,omitempty"`
	RoomType    string  `json:"room_type,omitempty"`
	RoomLinkID  string  `json:"room_link_id,omitempty"`
	SourceLogID *int64  `json:"source_log_id,omitempty"`
	ThreadID    *string `json:"thread_id,omitempty"`
	ThreadScope *int    `json:"thread_scope,omitempty"`
}
