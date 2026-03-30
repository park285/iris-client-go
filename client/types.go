package client

type ReplyRequest struct {
	Type        string  `json:"type"`
	Room        string  `json:"room"`
	Data        string  `json:"data"`
	ThreadID    *string `json:"threadId,omitempty"`
	ThreadScope *int    `json:"threadScope,omitempty"`
}

type replyImageMetadata struct {
	Type        string  `json:"type"`
	Room        string  `json:"room"`
	ThreadID    *string `json:"threadId,omitempty"`
	ThreadScope *int    `json:"threadScope,omitempty"`
}

type DecryptRequest struct {
	B64Ciphertext string `json:"b64_ciphertext"`
	UserID        *int64 `json:"user_id,omitempty"`
	Enc           int    `json:"enc"`
}

type DecryptResponse struct {
	PlainText string `json:"plain_text"`
}
