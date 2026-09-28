package webhook

import (
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"strconv"
	"sync"
)

type scheduledTask struct {
	task webhookTask
	key  string
}

type taskRunner func(index int, task webhookTask)

// schedulerShard의 slots는 수락했지만 아직 시작하지 않은 task 수를 shard 용량으로 제한한다.
// 수락 여부를 dispatcher goroutine의 스케줄링 시점이 아니라 이 용량만으로 정하므로, CPU 경합으로
// dispatcher가 늦게 돌아도 빈 queue를 queue full로 거절하지 않는다.
// Slot을 쥔 enqueue만 incoming에 보내고 incoming 버퍼가 slots 용량과 같아서, slot을 얻은 뒤의
// 전송은 막히지 않는다.
type schedulerShard struct {
	incoming chan webhookTask
	slots    chan struct{}
}

func newSchedulerShard(capacity int) schedulerShard {
	return schedulerShard{
		incoming: make(chan webhookTask, capacity),
		slots:    make(chan struct{}, capacity),
	}
}

type scheduler struct {
	queueSize    int
	orderingMode OrderingMode
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

		s.shards[i] = newSchedulerShard(shardQueue)
		s.startShard(&s.shards[i], shardWorkers, workerOffset, runner)

		workerOffset += shardWorkers
	}
}

func (s *scheduler) startShard(shard *schedulerShard, workerCount, workerOffset int, runner taskRunner) {
	work := make(chan scheduledTask)
	done := make(chan string, cap(shard.slots))

	if s.taskPool != nil {
		// crosscutting:allow 외부 TaskPool reject/panic은 relay fallback이 task를 정확히 한 번 실행하고 runner panic은 runScheduledTask가 key를 release한다.
		s.wg.Go(func() {
			for st := range work {
				key := st.key
				release := sync.OnceFunc(func() { done <- key })
				run := sync.OnceFunc(func() {
					s.runScheduledTask(0, st, shard.slots, release, runner)
				})

				if !s.submitTask(run) {
					run()
				}
			}
		})
	} else {
		for i := range workerCount {
			idx := workerOffset + i

			// crosscutting:allow runScheduledTask가 runner panic을 복구하고 dispatcher key를 반드시 release한다.
			s.wg.Go(func() {
				for st := range work {
					s.runScheduledTask(idx, st, shard.slots, func() { done <- st.key }, runner)
				}
			})
		}
	}

	// crosscutting:allow dispatcher는 외부 callback을 실행하지 않으며 panic recovery가 부분 inflight 상태를 숨기면 drain 교착이 된다.
	s.wg.Go(func() {
		defer close(work)

		runDispatcher(shard.incoming, work, done, s.orderingMode)
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

func (s *scheduler) runScheduledTask(index int, st scheduledTask, slots <-chan struct{}, release func(), runner taskRunner) {
	// 시작한 task는 queue 용량을 더 차지하지 않는다.
	<-slots

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

func (s *scheduler) stop() {
	for i := range s.shards {
		close(s.shards[i].incoming)
	}

	s.wg.Wait()
}

// pending은 수락했지만 아직 시작하지 않은 task 수를 반환한다.
func (s *scheduler) pending() int {
	total := 0

	for i := range s.shards {
		total += len(s.shards[i].slots)
	}

	return total
}

func (s *scheduler) shardFor(task webhookTask) *schedulerShard {
	if len(s.shards) == 0 {
		panic("scheduler not started")
	}

	index := schedulerShardIndex(stripeKey(task.msg), len(s.shards))

	return &s.shards[index]
}

func schedulerShardCount(workerCount, queueSize int) int {
	if workerCount <= 1 || queueSize <= 1 {
		return 1
	}

	if queueSize < workerCount {
		return queueSize
	}

	return workerCount
}

func schedulerShardIndex(key string, shardCount int) int {
	if shardCount <= 1 || shardCount > math.MaxInt32 || key == "" {
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

func runDispatcher(incoming <-chan webhookTask, work chan<- scheduledTask, done <-chan string, orderingMode OrderingMode) {
	state := dispatchState{
		inflight:     make(map[string]bool),
		pending:      make(map[string][]webhookTask),
		orderingMode: orderingMode,
	}
	inCh := incoming

	for {
		if inCh == nil && len(state.ready) == 0 && len(state.inflight) == 0 {
			return
		}

		var (
			workCh chan<- scheduledTask
			next   scheduledTask
		)

		if len(state.ready) > 0 {
			workCh = work
			next = state.ready[0]
		}

		select {
		case task, ok := <-inCh:
			if !ok {
				inCh = nil
				continue
			}

			state.admit(task)

		case workCh <- next:
			state.ready = state.ready[1:]

		case key := <-done:
			state.complete(key)
		}
	}
}

type dispatchState struct {
	ready        []scheduledTask
	inflight     map[string]bool
	pending      map[string][]webhookTask
	nextID       uint64
	orderingMode OrderingMode
}

// 같은 stripe key가 실행 중이면 pending 큐에 넣어 순서를 지키고, 아니면 바로 ready로 올린다.
func (s *dispatchState) admit(task webhookTask) {
	key := stripeKey(task.msg)

	if s.orderingMode == OrderingModeNone {
		s.nextID++

		key = "unordered:" + strconv.FormatUint(s.nextID, 10)
	}

	if s.inflight[key] {
		s.pending[key] = append(s.pending[key], task)
	} else {
		s.inflight[key] = true
		s.ready = append(s.ready, scheduledTask{task: task, key: key})
	}
}

func (s *dispatchState) complete(key string) {
	q := s.pending[key]
	if len(q) == 0 {
		delete(s.inflight, key)

		return
	}

	nextTask := q[0]

	s.pending[key] = q[1:]

	if len(s.pending[key]) == 0 {
		delete(s.pending, key)
	}

	s.ready = append(s.ready, scheduledTask{task: nextTask, key: key})
}
