package webhook

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
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
	queueSize    int
	orderingMode OrderingMode
	depth        atomic.Int32
	wg           sync.WaitGroup
	shards       []schedulerShard
	taskPool     TaskPool
	logger       *slog.Logger
}

func newScheduler(queueSize int, taskPool TaskPool, orderingMode OrderingMode, logger *slog.Logger) *scheduler {
	return &scheduler{
		queueSize:    queueSize,
		orderingMode: orderingMode,
		taskPool:     taskPool,
		logger:       resolveLogger(logger),
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
	started := make(chan struct{}, shard.maxBuffered+workerCount+1)

	if s.taskPool != nil {
		// crosscutting:allow 외부 TaskPool reject/panic은 relay fallback이 task를 정확히 한 번 실행하고 runner panic은 runScheduledTask가 key를 release한다.
		s.wg.Go(func() {
			for st := range work {
				key := st.key
				release := sync.OnceFunc(func() { done <- key })
				run := sync.OnceFunc(func() {
					s.runScheduledTask(0, st, started, release, runner)
				})
				if !s.submitTask(run) {
					run()
				}
			}
		})
	} else {
		for i := range workerCount {
			s.wg.Add(1)
			// crosscutting:allow runScheduledTask가 runner panic을 복구하고 dispatcher key를 반드시 release한다.
			go func(idx int) {
				defer s.wg.Done()
				for st := range work {
					s.runScheduledTask(idx, st, started, func() { done <- st.key }, runner)
				}
			}(workerOffset + i)
		}
	}

	// crosscutting:allow dispatcher는 외부 callback을 실행하지 않으며 panic recovery가 부분 inflight 상태를 숨기면 drain 교착이 된다.
	s.wg.Go(func() {
		defer close(work)
		runDispatcher(shard.incoming, work, started, done, shard.maxBuffered, s.orderingMode, &s.depth)
	})
}

func (s *scheduler) submitTask(task func()) (accepted bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error(
				"webhook_task_pool_panic_recovered",
				slog.String("panic_type", fmt.Sprintf("%T", recovered)),
				slog.String("stack", string(debug.Stack())),
			)
			accepted = false
		}
	}()

	return s.taskPool.SubmitWait(task)
}

func (s *scheduler) runScheduledTask(index int, st scheduledTask, started chan<- struct{}, release func(), runner taskRunner) {
	s.depth.Add(-1)
	started <- struct{}{}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error(
				"webhook_scheduler_runner_panic_recovered",
				slog.String("panic_type", fmt.Sprintf("%T", recovered)),
				slog.Int("worker", index),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()
	defer release()

	runner(index, st.task)
}

func (s *scheduler) close() {
	for i := range s.shards {
		close(s.shards[i].incoming)
	}
	s.wg.Wait()
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

	//nolint:gosec // G115: shardCount는 line 178에서 <=1이 걸러진 작은 양수(worker/queue 수 파생)라 uint32 변환이 wrap하지 않고, 나머지도 shardCount 미만이라 int 변환이 안전하다.
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

func runDispatcher(incoming <-chan webhookTask, work chan<- scheduledTask, started <-chan struct{}, done <-chan string, maxBuffered int, orderingMode OrderingMode, depth *atomic.Int32) {
	var (
		ready    []scheduledTask
		inflight = make(map[string]bool)
		pending  = make(map[string][]webhookTask)
		buffered int
		inCh     = incoming
		nextID   uint64
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
			if orderingMode == OrderingModeNone {
				nextID++
				key = "unordered:" + strconv.FormatUint(nextID, 10)
			}
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

		case <-started:
			buffered--

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
