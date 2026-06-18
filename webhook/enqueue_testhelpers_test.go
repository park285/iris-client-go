package webhook

import "context"

func (s *scheduler) enqueue(task webhookTask) {
	s.incomingFor(task) <- task
}

func (h *Handler) enqueue(task webhookTask) error {
	return h.enqueueTask(context.Background(), task)
}
