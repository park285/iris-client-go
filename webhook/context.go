package webhook

import (
	"bytes"
	"encoding/json"
	"strings"
)

// MessageContext는 webhook.Message의 정규화된 immutable snapshot입니다.
type MessageContext struct {
	route                 string
	roomID                string
	text                  string
	sender                string
	userID                string
	messageType           string
	threadID              string
	threadScope           int
	hasThreadScope        bool
	messageID             string
	chatLogID             string
	roomType              string
	roomLinkID            string
	sourceLogID           int64
	hasSourceLogID        bool
	rawSourceLogID        int64
	hasRawSourceLogID     bool
	sourceGenerationID    int64
	hasSourceGeneration   bool
	sourceAccountID       string
	isMine                bool
	hasIsMine             bool
	origin                string
	attachment            string
	mentions              []WebhookMention
	eventPayload          json.RawMessage
	eventType             string
	eventKind             string
	eventStatus           string
	eventSchemaVersion    int
	hasEventSchemaVersion bool
}

func NewMessageContext(message *Message) MessageContext {
	result := MessageContext{}
	if message == nil {
		return result
	}

	result.roomID = strings.TrimSpace(message.Room)
	result.text = message.Msg
	if message.Sender != nil {
		result.sender = strings.TrimSpace(*message.Sender)
	}
	if message.JSON == nil {
		return result
	}

	wire := message.JSON
	result.route = strings.TrimSpace(wire.Route)
	if value := strings.TrimSpace(wire.ChatID); value != "" {
		result.roomID = value
	}
	if strings.TrimSpace(wire.Message) != "" {
		result.text = wire.Message
	}
	result.userID = strings.TrimSpace(wire.UserID)
	result.messageType = strings.TrimSpace(wire.Type)
	if wire.ThreadID != nil {
		result.threadID = strings.TrimSpace(*wire.ThreadID)
	}
	if wire.ThreadScope != nil {
		result.threadScope = *wire.ThreadScope
		result.hasThreadScope = true
	}
	result.messageID = strings.TrimSpace(wire.MessageID)
	result.chatLogID = strings.TrimSpace(wire.ChatLogID)
	result.roomType = strings.TrimSpace(wire.RoomType)
	result.roomLinkID = strings.TrimSpace(wire.RoomLinkID)
	if wire.SourceLogID != nil {
		result.sourceLogID = *wire.SourceLogID
		result.hasSourceLogID = true
	}
	if wire.RawSourceLogID != nil {
		result.rawSourceLogID = *wire.RawSourceLogID
		result.hasRawSourceLogID = true
	}
	if wire.SourceGenerationID != nil {
		result.sourceGenerationID = *wire.SourceGenerationID
		result.hasSourceGeneration = true
	}
	result.sourceAccountID = strings.TrimSpace(wire.SourceAccountID)
	if wire.IsMine != nil {
		result.isMine = *wire.IsMine
		result.hasIsMine = true
	}
	result.origin = strings.TrimSpace(wire.Origin)
	result.attachment = wire.Attachment
	result.mentions = cloneWebhookMentions(wire.Mentions)
	result.eventPayload = append(json.RawMessage(nil), wire.EventPayload...)
	result.eventType, result.eventKind, result.eventStatus, result.eventSchemaVersion,
		result.hasEventSchemaVersion = semanticEventHeader(result.eventPayload)

	return result
}

func semanticEventHeader(raw json.RawMessage) (string, string, string, int, bool) {
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
	if err := json.Unmarshal(raw, &header); err != nil {
		return "", "", "", 0, false
	}
	if header.SchemaVersion == nil {
		return strings.TrimSpace(header.Type), strings.TrimSpace(header.Kind),
			strings.TrimSpace(header.Status), 0, false
	}
	return strings.TrimSpace(header.Type), strings.TrimSpace(header.Kind),
		strings.TrimSpace(header.Status), *header.SchemaVersion, true
}
