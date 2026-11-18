package common

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

type ReplyRequest struct {
	ClientRequestID *string        `json:"clientRequestId,omitempty"`
	Type            string         `json:"type"`
	Room            string         `json:"room"`
	Data            string         `json:"data"`
	ThreadID        *string        `json:"threadId,omitempty"`
	Mentions        []ReplyMention `json:"mentions,omitempty"`
}

type ReplyMention struct {
	UserID   any    `json:"userId"`
	Nickname string `json:"nickname,omitempty"`
	At       []int  `json:"at,omitempty"`
	Len      int    `json:"len,omitempty,omitzero"`
}

func (m ReplyMention) MarshalJSON() ([]byte, error) {
	userID, err := NormalizeReplyMentionUserID(m.UserID)
	if err != nil {
		return nil, fmt.Errorf("normalize mention userId: %w", err)
	}

	type wireMention ReplyMention

	m.UserID = userID

	data, err := jsonv2.Marshal(wireMention(m))
	if err != nil {
		return nil, fmt.Errorf("encode reply mention: %w", err)
	}

	return data, nil
}

func NormalizeReplyMentionUserID(value any) (any, error) {
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, errors.New("iris: mention userId must not be blank")
		}

		return trimmed, nil
	case int, int8, int16, int32, int64:
		signed := reflect.ValueOf(v).Int()
		if signed <= 0 {
			return nil, fmt.Errorf("iris: mention userId must be positive, got %d", signed)
		}

		return signed, nil
	case uint, uint8, uint16, uint32, uint64:
		unsigned := reflect.ValueOf(v).Uint()
		if unsigned == 0 || unsigned > math.MaxInt64 {
			return nil, fmt.Errorf("iris: mention userId must be positive, got %d", unsigned)
		}

		return int64(unsigned), nil
	case nil:
		return nil, errors.New("iris: mention userId is required")
	default:
		return nil, errors.New("iris: mention userId must be string or positive integer")
	}
}

type ImagePartSpec struct {
	Index       int    `json:"index"`
	SHA256Hex   string `json:"sha256Hex"`
	ByteLength  int64  `json:"byteLength"`
	ContentType string `json:"contentType"`
}

type ReplyImageMetadata struct {
	ClientRequestID *string         `json:"clientRequestId,omitempty"`
	Type            string          `json:"type"`
	Room            string          `json:"room"`
	ThreadID        *string         `json:"threadId,omitempty"`
	Images          []ImagePartSpec `json:"images"`
}
