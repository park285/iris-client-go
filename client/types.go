package client

type ReplyRequest struct {
	Type        string  `json:"type"`
	Room        string  `json:"room"`
	Data        string  `json:"data"`
	ThreadID    *string `json:"threadId,omitempty"`
	ThreadScope *int    `json:"threadScope,omitempty"`
}

type imagePartSpec struct {
	Index       int    `json:"index"`
	SHA256Hex   string `json:"sha256Hex"`
	ByteLength  int64  `json:"byteLength"`
	ContentType string `json:"contentType"`
}

type replyImageMetadata struct {
	Type        string          `json:"type"`
	Room        string          `json:"room"`
	ThreadID    *string         `json:"threadId,omitempty"`
	ThreadScope *int            `json:"threadScope,omitempty"`
	Images      []imagePartSpec `json:"images"`
}

type DecryptRequest struct {
	B64Ciphertext string `json:"b64_ciphertext"`
	UserID        *int64 `json:"user_id,omitempty"`
	Enc           int    `json:"enc"`
}

type DecryptResponse struct {
	PlainText string `json:"plain_text"`
}
