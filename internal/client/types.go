package client

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ReplyRequest struct {
	ClientRequestID *string        `json:"clientRequestId,omitempty"`
	Type            string         `json:"type"`
	Room            string         `json:"room"`
	Data            string         `json:"data"`
	ThreadID        *string        `json:"threadId,omitempty"`
	ThreadScope     *int           `json:"threadScope,omitempty"`
	Mentions        []ReplyMention `json:"mentions,omitempty"`
}

type ReplyMention struct {
	UserID   ReplyMentionUserID `json:"userId"`
	Nickname string             `json:"nickname,omitempty"`
	At       []int              `json:"at,omitempty"`
	Len      int                `json:"len,omitempty"`
}

type ReplyMentionUserID = any

func (m ReplyMention) MarshalJSON() ([]byte, error) {
	userID, err := normalizeReplyMentionUserID(m.UserID)
	if err != nil {
		return nil, err
	}
	type replyMentionJSON struct {
		UserID   ReplyMentionUserID `json:"userId"`
		Nickname string             `json:"nickname,omitempty"`
		At       []int              `json:"at,omitempty"`
		Len      int                `json:"len,omitempty"`
	}
	return json.Marshal(replyMentionJSON{
		UserID:   userID,
		Nickname: m.Nickname,
		At:       m.At,
		Len:      m.Len,
	})
}

func (m *ReplyMention) UnmarshalJSON(data []byte) error {
	type replyMentionJSON struct {
		UserID   json.RawMessage `json:"userId"`
		Nickname string          `json:"nickname,omitempty"`
		At       []int           `json:"at,omitempty"`
		Len      int             `json:"len,omitempty"`
	}
	var wire replyMentionJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	userID, err := parseReplyMentionUserID(wire.UserID)
	if err != nil {
		return err
	}
	m.UserID = userID
	m.Nickname = wire.Nickname
	m.At = wire.At
	m.Len = wire.Len
	return nil
}

func parseReplyMentionUserID(raw json.RawMessage) (ReplyMentionUserID, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil, fmt.Errorf("iris: mention userId is required")
	}
	if strings.HasPrefix(value, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		return normalizeReplyMentionUserID(text)
	}
	numeric, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("iris: mention userId must be string or positive integer")
	}
	return normalizeReplyMentionUserID(numeric)
}

func normalizeReplyMentionUserID(value ReplyMentionUserID) (ReplyMentionUserID, error) {
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, fmt.Errorf("iris: mention userId must not be blank")
		}
		return trimmed, nil
	case int:
		return normalizeSignedReplyMentionUserID(int64(v))
	case int8:
		return normalizeSignedReplyMentionUserID(int64(v))
	case int16:
		return normalizeSignedReplyMentionUserID(int64(v))
	case int32:
		return normalizeSignedReplyMentionUserID(int64(v))
	case int64:
		return normalizeSignedReplyMentionUserID(v)
	case uint:
		return normalizeUnsignedReplyMentionUserID(uint64(v))
	case uint8:
		return normalizeUnsignedReplyMentionUserID(uint64(v))
	case uint16:
		return normalizeUnsignedReplyMentionUserID(uint64(v))
	case uint32:
		return normalizeUnsignedReplyMentionUserID(uint64(v))
	case uint64:
		return normalizeUnsignedReplyMentionUserID(v)
	case json.Number:
		numeric, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("iris: mention userId must be string or positive integer")
		}
		return normalizeSignedReplyMentionUserID(numeric)
	case nil:
		return nil, fmt.Errorf("iris: mention userId is required")
	default:
		return nil, fmt.Errorf("iris: mention userId must be string or positive integer")
	}
}

func normalizeSignedReplyMentionUserID(value int64) (ReplyMentionUserID, error) {
	if value <= 0 {
		return nil, fmt.Errorf("iris: mention userId must be positive, got %d", value)
	}
	return value, nil
}

func normalizeUnsignedReplyMentionUserID(value uint64) (ReplyMentionUserID, error) {
	if value == 0 || value > math.MaxInt64 {
		return nil, fmt.Errorf("iris: mention userId must be positive, got %d", value)
	}
	return int64(value), nil
}

type imagePartSpec struct {
	Index       int    `json:"index"`
	SHA256Hex   string `json:"sha256Hex"`
	ByteLength  int64  `json:"byteLength"`
	ContentType string `json:"contentType"`
}

type replyImageMetadata struct {
	ClientRequestID *string         `json:"clientRequestId,omitempty"`
	Type            string          `json:"type"`
	Room            string          `json:"room"`
	ThreadID        *string         `json:"threadId,omitempty"`
	ThreadScope     *int            `json:"threadScope,omitempty"`
	Images          []imagePartSpec `json:"images"`
}
