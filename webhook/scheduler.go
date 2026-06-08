package webhook

import (
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
	taskPool  TaskPool
}

func newScheduler(queueSize int, taskPool TaskPool) *scheduler {
	return &scheduler{
		queueSize: queueSize,
		taskPool:  taskPool,
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
	done := make(chan string, shard.maxBuffered)

	if s.taskPool != nil {
		s.wg.Go(func() {
			for st := range work {
				key := st.key
				task := st.task
				if !s.taskPool.SubmitWait(func() {
					runner(0, task)
					done <- key
				}) {
					// pool 종료로 콜백이 실행되지 않으면 done<-key가 누락되어 dispatcher의 inflight가 영구 잔류한다.
					// 이 key와 잔여 work의 key를 모두 release해야 runDispatcher가 종료되고 Close()가 hang하지 않는다.
					done <- key
					for st := range work {
						done <- st.key
					}

					return
				}
			}
		})
	} else {
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
	}

	s.wg.Go(func() {
		defer close(work)
		runDispatcher(shard.incoming, work, done, shard.maxBuffered, &s.depth)
	})
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

	return int(fnv32aString(key) % uint32(shardCount))
}

const (
	fnv32aOffset = uint32(2166136261)
	fnv32aPrime  = uint32(16777619)
)

func fnv32aString(value string) uint32 {
	hash := fnv32aOffset
	for i := range len(value) {
		hash ^= uint32(value[i])
		hash *= fnv32aPrime
	}
	return hash
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
