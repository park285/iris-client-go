package common

type ReplyAcceptedResponse struct {
	Success   bool   `json:"success"`
	Delivery  string `json:"delivery"`
	RequestID string `json:"requestId"`
	Room      string `json:"room"`
	Type      string `json:"type"`
	Duplicate *bool  `json:"duplicate,omitempty"`
}

// ReplyStatusSnapshot은 GET /reply-status/{requestId} 응답입니다.
//
// State는 Iris reply lifecycle의 wire 값인 queued, preparing, prepared, sending,
// handoff_completed, outcome_unknown, failed 중 하나입니다. 이 중 outcome_unknown은 외부 전송 결과를
// 확인하지 못한 상태로, 발신 실패가 증명되지 않았으므로 failed처럼 재전송하면 중복 발신될 수 있습니다
// (DEC-20260731-reply-outcome-unknown-fail-closed). Iris는 알 수 없는 내부 admission 상태도
// outcome_unknown으로 보고합니다. 새 상태가 추가될 수 있으므로 소비자는 모르는 값을 성공이나
// 실패로 추정하지 말고 결과 불명으로 다뤄야 합니다.
type ReplyStatusSnapshot struct {
	RequestID        string  `json:"requestId"`
	State            string  `json:"state"`
	UpdatedAtEpochMs int64   `json:"updatedAtEpochMs"`
	Detail           *string `json:"detail,omitempty"`
}
