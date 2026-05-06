package client

type ReplyRequest struct {
	Type        string         `json:"type"`
	Room        string         `json:"room"`
	Data        string         `json:"data"`
	ThreadID    *string        `json:"threadId,omitempty"`
	ThreadScope *int           `json:"threadScope,omitempty"`
	Mentions    []ReplyMention `json:"mentions,omitempty"`
}

type ReplyMention struct {
	UserID   int64  `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty"`
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
