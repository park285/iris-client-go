package irisx

type ReplyRequest struct {
	Type     string  `json:"type"`
	Room     string  `json:"room"`
	Data     string  `json:"data"`
	ThreadID *string `json:"threadId,omitempty"`
}

type WebhookRequest struct {
	Text     string `json:"text"`
	Room     string `json:"room"`
	Sender   string `json:"sender"`
	UserID   string `json:"userId"`
	ThreadID string `json:"threadId"`
}
