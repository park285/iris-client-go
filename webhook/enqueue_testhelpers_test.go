package webhook

import "context"

func (s *scheduler) enqueue(task webhookTask) {
	s.incomingFor(task) <- task
}

func (h *Handler) enqueue(task webhookTask) error {
	return h.enqueueTask(context.Background(), task) //nolint:wrapcheck // 테스트 헬퍼는 enqueue 오류를 그대로 돌려 검사한다.
}
