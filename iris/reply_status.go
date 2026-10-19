package iris

// ReplyStatusClass는 관측한 전송 상태를 분류하며 수신 완료·재시도 안전성을 뜻하지 않는다.
type ReplyStatusClass string

const (
	// ReplyStatusPending은 대기·진행 중이며 인계가 끝나지 않은 상태다.
	ReplyStatusPending ReplyStatusClass = "pending"
	// ReplyStatusHandoffCompleted는 인계 완료이며 수신자의 수신 완료를 뜻하지 않는다.
	ReplyStatusHandoffCompleted ReplyStatusClass = "handoff_completed"
	// ReplyStatusFailed는 관측된 실패이며 재시도가 안전하다는 근거가 아니다.
	ReplyStatusFailed ReplyStatusClass = "failed"
	// ReplyStatusUnknown은 outcome_unknown과 알 수 없는 전송 상태를 포함한다.
	ReplyStatusUnknown ReplyStatusClass = "outcome_unknown"
)

// ClassifyReplyStatus는 Iris 전송 상태를 정규화 없이 정확히 대조한다.
// 빈 값·미지 상태는 ReplyStatusUnknown으로 분류한다. nil 스냅샷·요청 오류·폴링·취소·재시도·재발급은 호출자 책임이다.
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
