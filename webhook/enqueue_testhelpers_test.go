package webhook

import "context"

func (s *scheduler) enqueue(task webhookTask) {
	shard := s.shardFor(task)
	shard.slots <- struct{}{}

	shard.incoming <- task
}

func (h *Handler) enqueue(task webhookTask) error {
	return h.enqueueTask(context.Background(), task)
}
