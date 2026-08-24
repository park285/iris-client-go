package webhook

import (
	"bytes"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
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
	eventPayload          jsonv1.RawMessage
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

	applyWireIdentity(&result, message.JSON)
	applyWireSource(&result, message.JSON)
	applyWirePayload(&result, message.JSON)

	return result
}

func applyWireIdentity(m *MessageContext, wire *MessageJSON) {
	m.route = strings.TrimSpace(wire.Route)

	if value := strings.TrimSpace(wire.ChatID); value != "" {
		m.roomID = value
	}

	if strings.TrimSpace(wire.Message) != "" {
		m.text = wire.Message
	}

	m.userID = strings.TrimSpace(wire.UserID)
	m.messageType = strings.TrimSpace(wire.Type)

	if wire.ThreadID != nil {
		m.threadID = strings.TrimSpace(*wire.ThreadID)
	}

	if wire.ThreadScope != nil {
		m.threadScope = *wire.ThreadScope
		m.hasThreadScope = true
	}

	m.messageID = strings.TrimSpace(wire.MessageID)
	m.chatLogID = strings.TrimSpace(wire.ChatLogID)
	m.roomType = strings.TrimSpace(wire.RoomType)
	m.roomLinkID = strings.TrimSpace(wire.RoomLinkID)
}

func applyWireSource(m *MessageContext, wire *MessageJSON) {
	if wire.SourceLogID != nil {
		m.sourceLogID = *wire.SourceLogID
		m.hasSourceLogID = true
	}

	if wire.RawSourceLogID != nil {
		m.rawSourceLogID = *wire.RawSourceLogID
		m.hasRawSourceLogID = true
	}

	if wire.SourceGenerationID != nil {
		m.sourceGenerationID = *wire.SourceGenerationID
		m.hasSourceGeneration = true
	}

	m.sourceAccountID = strings.TrimSpace(wire.SourceAccountID)
	if wire.IsMine != nil {
		m.isMine = *wire.IsMine
		m.hasIsMine = true
	}
}

func applyWirePayload(m *MessageContext, wire *MessageJSON) {
	m.origin = strings.TrimSpace(wire.Origin)
	m.attachment = wire.Attachment
	m.mentions = cloneWebhookMentions(wire.Mentions)
	m.eventPayload = append(jsonv1.RawMessage(nil), wire.EventPayload...)
	m.eventType, m.eventKind, m.eventStatus, m.eventSchemaVersion,
		m.hasEventSchemaVersion = semanticEventHeader(m.eventPayload)
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
