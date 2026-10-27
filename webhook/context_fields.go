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
