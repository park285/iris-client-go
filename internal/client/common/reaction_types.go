package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Reaction은 Iris가 지원하는 KakaoTalk 표준 reaction입니다.
type Reaction string

const (
	ReactionHeart    Reaction = "heart"
	ReactionLike     Reaction = "like"
	ReactionCheck    Reaction = "check"
	ReactionLaugh    Reaction = "laugh"
	ReactionSurprise Reaction = "surprise"
	ReactionSad      Reaction = "sad"
)

// ReactionStatus는 Iris reaction admission/bridge 경로의 terminal status입니다.
type ReactionStatus string

const (
	ReactionStatusSent           ReactionStatus = "sent"
	ReactionStatusFailed         ReactionStatus = "failed"
	ReactionStatusOutcomeUnknown ReactionStatus = "outcome_unknown"
)

// ReactionRequest는 room log 하나에 대한 idempotent reaction 요청입니다.
// client는 add-only 또는 add 없이 follow/remove 조합만 허용합니다.
type ReactionRequest struct {
	RequestID string     `json:"requestId"`
	ChatLogID string     `json:"chatLogId"`
	LinkID    *int64     `json:"linkId,omitempty"`
	Revision  int64      `json:"revision"`
	Add       []Reaction `json:"add,omitempty"`
	Follow    []Reaction `json:"follow,omitempty"`
	Remove    []Reaction `json:"remove,omitempty"`
}

// ReactionResponse는 reaction admission endpoint의 응답입니다.
type ReactionResponse struct {
	Success   bool           `json:"success"`
	Status    ReactionStatus `json:"status"`
	Message   *string        `json:"message,omitempty"`
	RequestID string         `json:"requestId"`
	Duplicate *bool          `json:"duplicate,omitempty"`
}

// UnmarshalJSON keeps the reaction response contract closed at the client boundary.
func (response *ReactionResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		Success   *bool
		Status    *ReactionStatus
		Message   *string
		RequestID *string
		Duplicate *bool
	}

	err := decodeStrictJSONObject(data, map[string]struct{}{
		"success":   {},
		"status":    {},
		"message":   {},
		"requestId": {},
		"duplicate": {},
	}, func(field string, value json.RawMessage) error {
		switch field {
		case "success":
			return decodeRequiredJSONValue(value, "success", &wire.Success)
		case "status":
			return decodeRequiredJSONValue(value, "status", &wire.Status)
		case "message":
			return decodeRequiredJSONValue(value, "message", &wire.Message)
		case "requestId":
			return decodeRequiredJSONValue(value, "requestId", &wire.RequestID)
		case "duplicate":
			return decodeRequiredJSONValue(value, "duplicate", &wire.Duplicate)
		default:
			return fmt.Errorf("decode reaction response: unknown field %q", field)
		}
	})
	if err != nil {
		return fmt.Errorf("decode reaction response: %w", err)
	}
	if wire.Success == nil || wire.Status == nil || wire.RequestID == nil {
		return errors.New("decode reaction response: required field missing")
	}
	if strings.TrimSpace(*wire.RequestID) == "" {
		return errors.New("decode reaction response: requestId must not be blank")
	}
	switch *wire.Status {
	case ReactionStatusSent:
		if !*wire.Success {
			return errors.New("decode reaction response: sent status requires success=true")
		}
	case ReactionStatusFailed, ReactionStatusOutcomeUnknown:
		if *wire.Success {
			return fmt.Errorf("decode reaction response: %s status requires success=false", *wire.Status)
		}
	default:
		return fmt.Errorf("decode reaction response: unsupported status %q", *wire.Status)
	}

	*response = ReactionResponse{
		Success:   *wire.Success,
		Status:    *wire.Status,
		Message:   wire.Message,
		RequestID: *wire.RequestID,
		Duplicate: wire.Duplicate,
	}
	return nil
}
