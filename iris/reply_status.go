package iris

// ReplyStatusClass classifies an observed wire state, not delivery or retry safety.
type ReplyStatusClass string

const (
	// ReplyStatusPending means work is queued or in progress, not handed off.
	ReplyStatusPending ReplyStatusClass = "pending"
	// ReplyStatusHandoffCompleted means handoff completed, not recipient delivery.
	ReplyStatusHandoffCompleted ReplyStatusClass = "handoff_completed"
	// ReplyStatusFailed is an observed failure, not proof that retry is safe.
	ReplyStatusFailed ReplyStatusClass = "failed"
	// ReplyStatusUnknown includes outcome_unknown and unrecognized wire states.
	ReplyStatusUnknown ReplyStatusClass = "outcome_unknown"
)

// ClassifyReplyStatus matches exact Iris wire states without normalization.
// Empty and future states fail closed to ReplyStatusUnknown. Callers own nil
// snapshots, request errors, polling, cancellation, retry and reissue policy.
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
