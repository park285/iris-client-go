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
	UserID   ReplyMentionUserID `json:"userId"`
	Nickname string             `json:"nickname,omitempty"`
	At       []int              `json:"at,omitempty"`
	Len      int                `json:"len,omitempty,omitzero"`
}

type ReplyMentionUserID = any

func (m ReplyMention) MarshalJSON() ([]byte, error) {
	userID, err := normalizeReplyMentionUserID(m.UserID)
	if err != nil {
		return nil, fmt.Errorf("normalize mention userId: %w", err)
	}

	type replyMentionJSON struct {
		UserID   ReplyMentionUserID `json:"userId"`
		Nickname string             `json:"nickname,omitempty"`
		At       []int              `json:"at,omitempty"`
		Len      int                `json:"len,omitempty,omitzero"`
	}

	data, err := jsonv2.Marshal(replyMentionJSON{
		UserID:   userID,
		Nickname: m.Nickname,
		At:       m.At,
		Len:      m.Len,
	})
	if err != nil {
		return nil, fmt.Errorf("encode reply mention: %w", err)
	}

	return data, nil
}

func normalizeReplyMentionUserID(value ReplyMentionUserID) (ReplyMentionUserID, error) {
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

func NormalizeReplyMentionUserID(value ReplyMentionUserID) (ReplyMentionUserID, error) {
	return normalizeReplyMentionUserID(value)
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
