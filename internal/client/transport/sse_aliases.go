package transport

import "github.com/park285/iris-client-go/internal/client/sse"

type MemberNicknameUpdatedEvent = sse.MemberNicknameUpdatedEvent
type RawSSEEvent = sse.RawSSEEvent
type SSERoomEventBody = sse.SSERoomEventBody
type SSEStreamState = sse.SSEStreamState

const (
	EventTypeMemberNicknameUpdated    = sse.EventTypeMemberNicknameUpdated
	SSEEventRoomEvent                 = sse.SSEEventRoomEvent
	SSEEventStreamState               = sse.SSEEventStreamState
	StreamCursorStatusCurrent         = sse.StreamCursorStatusCurrent
	StreamCursorStatusStale           = sse.StreamCursorStatusStale
	StreamCursorStatusFuture          = sse.StreamCursorStatusFuture
	StreamRecoveryQueryRecentMessages = sse.StreamRecoveryQueryRecentMessages
)
