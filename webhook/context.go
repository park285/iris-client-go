package webhook

import (
	"bytes"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"strings"
)

// MessageContext는 webhook.Message의 정규화된 immutable snapshot입니다. 소비자의 feed 경로가
// 읽는 route·방·event header만 담는다.
type MessageContext struct {
	route                 string
	roomID                string
	messageType           string
	eventPayload          jsonv1.RawMessage
	eventType             string
	eventKind             string
	eventStatus           string
	eventSchemaVersion    int
	hasEventSchemaVersion bool
}

// NewMessageContext는 Message를 읽기 전용 snapshot으로 만든다.
func NewMessageContext(message *Message) MessageContext {
	result := MessageContext{}

	if message == nil {
		return result
	}

	result.roomID = strings.TrimSpace(message.Room)

	if message.JSON == nil {
		return result
	}

	result.route = strings.TrimSpace(message.JSON.Route)
	result.messageType = strings.TrimSpace(message.JSON.Type)
	result.eventPayload = append(jsonv1.RawMessage(nil), message.JSON.EventPayload...)
	result.eventType, result.eventKind, result.eventStatus, result.eventSchemaVersion,
		result.hasEventSchemaVersion = semanticEventHeader(result.eventPayload)

	return result
}

func semanticEventHeader(raw jsonv1.RawMessage) (string, string, string, int, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", "", "", 0, false
	}

	var header struct {
		Type          string `json:"type"`
		Kind          string `json:"kind"`
		Status        string `json:"status"`
		SchemaVersion *int   `json:"schemaVersion"`
	}

	if err := jsonv2.Unmarshal(raw, &header); err != nil {
		return "", "", "", 0, false
	}

	if header.SchemaVersion == nil {
		return strings.TrimSpace(header.Type), strings.TrimSpace(header.Kind),
			strings.TrimSpace(header.Status), 0, false
	}

	return strings.TrimSpace(header.Type), strings.TrimSpace(header.Kind),
		strings.TrimSpace(header.Status), *header.SchemaVersion, true
}
