package webhook

import (
	jsonv1 "encoding/json"
)

func (c MessageContext) Route() string {
	return c.route
}

func (c MessageContext) RoomID() string {
	return c.roomID
}

func (c MessageContext) Text() string {
	return c.text
}

func (c MessageContext) Sender() string {
	return c.sender
}

func (c MessageContext) UserID() string {
	return c.userID
}

func (c MessageContext) MessageType() string {
	return c.messageType
}

func (c MessageContext) IsText() bool {
	return c.messageType == "" || c.messageType == MessageTypeText
}

func (c MessageContext) RoomType() string {
	return c.roomType
}

func (c MessageContext) RoomLinkID() string {
	return c.roomLinkID
}

func (c MessageContext) ThreadID() string {
	return c.threadID
}

func (c MessageContext) ThreadScope() (int, bool) {
	return c.threadScope, c.hasThreadScope
}

func (c MessageContext) MessageID() string {
	return c.messageID
}

func (c MessageContext) ChatLogID() string {
	return c.chatLogID
}

func (c MessageContext) SourceLogID() (int64, bool) {
	return c.sourceLogID, c.hasSourceLogID
}

func (c MessageContext) RawSourceLogID() (int64, bool) {
	return c.rawSourceLogID, c.hasRawSourceLogID
}

func (c MessageContext) SourceGenerationID() (int64, bool) {
	return c.sourceGenerationID, c.hasSourceGeneration
}

func (c MessageContext) SourceAccountID() string {
	return c.sourceAccountID
}

func (c MessageContext) IsMine() (bool, bool) {
	return c.isMine, c.hasIsMine
}

func (c MessageContext) Origin() string {
	return c.origin
}

func (c MessageContext) Attachment() string {
	return c.attachment
}

func (c MessageContext) Mentions() []WebhookMention {
	return cloneWebhookMentions(c.mentions)
}

func (c MessageContext) EventPayload() jsonv1.RawMessage {
	return append(jsonv1.RawMessage(nil), c.eventPayload...)
}

func (c MessageContext) EventType() string {
	if c.eventType != "" {
		return c.eventType
	}

	return c.messageType
}

func (c MessageContext) EventKind() string {
	return c.eventKind
}

func (c MessageContext) EventStatus() string {
	return c.eventStatus
}

func (c MessageContext) EventSchemaVersion() (int, bool) {
	return c.eventSchemaVersion, c.hasEventSchemaVersion
}
