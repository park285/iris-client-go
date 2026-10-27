package iris

import "time"

// Iris reply admission 원장의 보존 기간과, 저장된 clientRequestId로 자동 재발송을 허용하는 지평이다.
// 값은 Iris DEFAULT_REPLY_ADMISSION_RETENTION(native/iris-runtime/src/reply/admission_store.rs)과 같다.
// 지평은 보존보다 ReplyReplayHorizonMargin만큼 짧아, 지평 끝에서 재발송한 요청도 Iris가 같은 id를
// 아직 기억한다.
const (
	ReplyAdmissionRetention     = 168 * time.Hour
	ReplyReplayHorizonMargin    = 24 * time.Hour
	ReplyAutomaticReplayHorizon = ReplyAdmissionRetention - ReplyReplayHorizonMargin
)
