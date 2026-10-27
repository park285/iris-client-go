package webhook

import "encoding/json/jsontext"

func (c MessageContext) Route() string {
	return c.route
}

func (c MessageContext) RoomID() string {
	return c.roomID
}

func (c MessageContext) EventPayload() jsontext.Value {
	return append(jsontext.Value(nil), c.eventPayload...)
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
