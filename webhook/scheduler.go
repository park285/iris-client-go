package webhook

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

type scheduledTask struct {
	task webhookTask
	key  string
}

type taskRunner func(index int, task webhookTask)

type schedulerShard struct {
	incoming    chan webhookTask
	maxBuffered int
}

type scheduler struct {
	queueSize int
	depth     atomic.Int32
	wg        sync.WaitGroup
	shards    []schedulerShard
}

func newScheduler(queueSize int) *scheduler {
	return &scheduler{
		queueSize: queueSize,
	}
}

func (s *scheduler) start(workerCount int, runner taskRunner) {
	shardCount := schedulerShardCount(workerCount, s.queueSize)
	s.shards = make([]schedulerShard, shardCount)

	workerBase := workerCount / shardCount
	workerRemainder := workerCount % shardCount
	queueBase := s.queueSize / shardCount
	queueRemainder := s.queueSize % shardCount
	workerOffset := 0

	for i := range shardCount {
		shardWorkers := workerBase
		if i < workerRemainder {
			shardWorkers++
		}

		shardQueue := queueBase
		if i < queueRemainder {
			shardQueue++
		}

		s.shards[i] = schedulerShard{
			incoming:    make(chan webhookTask),
			maxBuffered: shardQueue,
		}
		s.startShard(&s.shards[i], shardWorkers, workerOffset, runner)
		workerOffset += shardWorkers
	}
}

func (s *scheduler) startShard(shard *schedulerShard, workerCount int, workerOffset int, runner taskRunner) {
	work := make(chan scheduledTask)
	done := make(chan string, workerCount)

	for i := range workerCount {
		s.wg.Add(1)
		go func(idx int) {
			defer s.wg.Done()
			for st := range work {
				runner(idx, st.task)
				done <- st.key
			}
		}(workerOffset + i)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(work)
		runDispatcher(shard.incoming, work, done, shard.maxBuffered, &s.depth)
	}()
}

func (s *scheduler) close() {
	for i := range s.shards {
		close(s.shards[i].incoming)
	}
	s.wg.Wait()
}

func (s *scheduler) enqueue(task webhookTask) {
	s.incomingFor(task) <- task
}

func (s *scheduler) incomingFor(task webhookTask) chan webhookTask {
	return s.shardFor(task).incoming
}

func (s *scheduler) shardFor(task webhookTask) *schedulerShard {
	if len(s.shards) == 0 {
		panic("scheduler not started")
	}

	index := schedulerShardIndex(stripeKey(task.msg), len(s.shards))
	return &s.shards[index]
}

func schedulerShardCount(workerCount int, queueSize int) int {
	if workerCount <= 1 || queueSize <= 1 {
		return 1
	}

	if queueSize < workerCount {
		return queueSize
	}

	return workerCount
}

func schedulerShardIndex(key string, shardCount int) int {
	if shardCount <= 1 || key == "" {
		return 0
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))

	return int(hasher.Sum32() % uint32(shardCount))
}

func runDispatcher(incoming <-chan webhookTask, work chan<- scheduledTask, done <-chan string, maxBuffered int, depth *atomic.Int32) {
	var (
		ready    []scheduledTask
		inflight = make(map[string]bool)
		pending  = make(map[string][]webhookTask)
		buffered int
		inCh     = incoming
	)

	for {
		if inCh == nil && len(ready) == 0 && len(inflight) == 0 {
			return
		}

		effectiveInCh := inCh
		if buffered >= maxBuffered {
			effectiveInCh = nil
		}

		var workCh chan<- scheduledTask
		var next scheduledTask
		if len(ready) > 0 {
			workCh = work
			next = ready[0]
		}

		select {
		case task, ok := <-effectiveInCh:
			if !ok {
				inCh = nil
				continue
			}
			key := stripeKey(task.msg)
			if inflight[key] {
				pending[key] = append(pending[key], task)
			} else {
				inflight[key] = true
				ready = append(ready, scheduledTask{task: task, key: key})
			}
			buffered++
			depth.Add(1)

		case workCh <- next:
			ready = ready[1:]
			buffered--
			depth.Add(-1)

		case key := <-done:
			if q := pending[key]; len(q) > 0 {
				nextTask := q[0]
				pending[key] = q[1:]
				if len(pending[key]) == 0 {
					delete(pending, key)
				}
				ready = append(ready, scheduledTask{task: nextTask, key: key})
			} else {
				delete(inflight, key)
			}
		}
	}
}
