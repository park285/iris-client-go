package iris

type ReplyStatusClass string

const (
	ReplyStatusPending          ReplyStatusClass = "pending"
	ReplyStatusHandoffCompleted ReplyStatusClass = "handoff_completed"
	ReplyStatusFailed           ReplyStatusClass = "failed"
	ReplyStatusUnknown          ReplyStatusClass = "outcome_unknown"
)

func ClassifyReplyStatus(state string) ReplyStatusClass {
	switch state {
	case "queued", "preparing", "prepared", "sending":
		return ReplyStatusPending
	case "handoff_completed":
		return ReplyStatusHandoffCompleted
	case "failed":
		return ReplyStatusFailed
	default:
		return ReplyStatusUnknown
	}
}

const ReplyFailureReasonRoomNotJoined = "room_not_joined"

func ReplyStatusRoomNotJoined(snapshot *ReplyStatusSnapshot) bool {
	return snapshot != nil &&
		snapshot.State == string(ReplyStatusFailed) &&
		snapshot.Reason != nil &&
		*snapshot.Reason == ReplyFailureReasonRoomNotJoined
}
