package webhook

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingTaskPool struct {
	runTasks  bool
	submits   chan func()
	calls     atomic.Int32
	stopCalls atomic.Int32
}

func (p *recordingTaskPool) SubmitWait(task func()) bool {
	p.calls.Add(1)
	if p.submits != nil {
		p.submits <- task
	}
	if p.runTasks && task != nil {
		task()
	}

	return true
}

func (p *recordingTaskPool) StopAndWait() {
	p.stopCalls.Add(1)
}

func TestSchedulerPreservesPerKeyOrder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen []string

	sched := newScheduler(100, nil, OrderingModeKey)
	sched.start(2, func(_ int, task webhookTask) {
		time.Sleep(time.Millisecond)
		mu.Lock()
		seen = append(seen, task.msg.Msg)
		mu.Unlock()
	})

	threadA := "a"
	for i := range 5 {
		msg := &Message{Room: "room", Msg: fmt.Sprintf("a-%d", i), JSON: &MessageJSON{ThreadID: &threadA}}
		sched.enqueue(webhookTask{msg: msg})
	}

	sched.close()

	mu.Lock()
	defer mu.Unlock()

	var aOrder []string
	for _, s := range seen {
		if s[0] == 'a' {
			aOrder = append(aOrder, s)
		}
	}

	for i := 1; i < len(aOrder); i++ {
		if aOrder[i-1] >= aOrder[i] {
			t.Fatalf("key-a out of order: %v", aOrder)
		}
	}
}

func TestSchedulerProcessesConcurrentKeys(t *testing.T) {
	t.Parallel()

	var processed atomic.Int32

	sched := newScheduler(100, nil, OrderingModeKey)
	sched.start(4, func(_ int, _ webhookTask) {
		processed.Add(1)
	})

	threadA := "a"
	threadB := "b"
	for range 10 {
		sched.enqueue(webhookTask{msg: &Message{Room: "r", Msg: "a", JSON: &MessageJSON{ThreadID: &threadA}}})
		sched.enqueue(webhookTask{msg: &Message{Room: "r", Msg: "b", JSON: &MessageJSON{ThreadID: &threadB}}})
	}

	sched.close()

	if processed.Load() != 20 {
		t.Fatalf("processed = %d, want 20", processed.Load())
	}
}

func TestSchedulerCloseWaitsForDrain(t *testing.T) {
	t.Parallel()

	var processed atomic.Int32
	block := make(chan struct{})

	sched := newScheduler(10, nil, OrderingModeKey)
	sched.start(1, func(_ int, _ webhookTask) {
		processed.Add(1)
		<-block
	})

	sched.enqueue(webhookTask{msg: &Message{Msg: "first"}})
	sched.enqueue(webhookTask{msg: &Message{Msg: "second"}})

	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		sched.close()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("close returned before drain")
	case <-time.After(50 * time.Millisecond):
	}

	close(block)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close did not return after drain")
	}

	if processed.Load() != 2 {
		t.Fatalf("processed = %d, want 2", processed.Load())
	}
}

func TestSchedulerCapacityBound(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	var received atomic.Int32

	queueSize := 3
	sched := newScheduler(queueSize, nil, OrderingModeKey)
	sched.start(1, func(_ int, _ webhookTask) {
		received.Add(1)
		<-block
	})

	threadA := "a"

	// 1번째: worker에서 처리 중 (block)
	first := webhookTask{msg: &Message{Room: "r", Msg: "1", JSON: &MessageJSON{ThreadID: &threadA}}}
	sched.enqueue(first)
	time.Sleep(10 * time.Millisecond)

	// 2~4번째: dispatcher pending에 적재 (buffered = queueSize = 3)
	for i := range queueSize {
		sched.enqueue(webhookTask{msg: &Message{Room: "r", Msg: fmt.Sprintf("%d", i+2), JSON: &MessageJSON{ThreadID: &threadA}}})
	}

	// dispatcher 내부 buffered가 maxBuffered에 도달하여 incoming 읽기를 중단해야 함
	// 추가 전송 시도는 즉시 실패해야 함
	overflow := webhookTask{msg: &Message{Room: "r", Msg: "overflow", JSON: &MessageJSON{ThreadID: &threadA}}}
	select {
	case sched.incomingFor(overflow) <- overflow:
		t.Fatal("send should block when capacity is reached")
	case <-time.After(50 * time.Millisecond):
	}

	close(block)
	sched.close()
}

func TestStartShard_WithTaskPool_RelayMode(t *testing.T) {
	t.Parallel()

	pool := &recordingTaskPool{
		runTasks: true,
		submits:  make(chan func(), 2),
	}
	sched := newScheduler(2, pool, OrderingModeKey)
	sched.shards = []schedulerShard{{
		incoming:    make(chan webhookTask),
		maxBuffered: 2,
	}}

	var processed atomic.Int32
	sched.startShard(&sched.shards[0], 3, 10, func(index int, _ webhookTask) {
		if index != 0 {
			t.Errorf("runner index = %d, want relay index 0", index)
		}
		processed.Add(1)
	})

	for i := range 2 {
		sched.enqueue(webhookTask{msg: &Message{Room: fmt.Sprintf("room-%d", i)}})
	}
	sched.close()

	if got := pool.calls.Load(); got != 2 {
		t.Fatalf("SubmitWait calls = %d, want 2", got)
	}
	if got := processed.Load(); got != 2 {
		t.Fatalf("processed tasks = %d, want 2", got)
	}
}

type rejectingTaskPool struct {
	calls atomic.Int32
}

func (p *rejectingTaskPool) SubmitWait(task func()) bool {
	p.calls.Add(1)

	return false
}

func (p *rejectingTaskPool) StopAndWait() {}

func TestStartShard_SubmitWaitFalseDoesNotHangClose(t *testing.T) {
	t.Parallel()

	pool := &rejectingTaskPool{}
	sched := newScheduler(2, pool, OrderingModeKey)
	sched.shards = []schedulerShard{{
		incoming:    make(chan webhookTask),
		maxBuffered: 2,
	}}
	sched.startShard(&sched.shards[0], 1, 0, func(_ int, _ webhookTask) {})

	sched.shards[0].incoming <- webhookTask{msg: &Message{Room: "r"}}

	done := make(chan struct{})
	go func() {
		sched.close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sched.close() hung when SubmitWait returned false")
	}

	if pool.calls.Load() == 0 {
		t.Fatal("SubmitWait was never called")
	}
}

func TestStartShard_DoneBufferSize(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatalf("ReadFile(scheduler.go) error = %v", err)
	}

	text := string(source)
	if !strings.Contains(text, "done := make(chan string, shard.maxBuffered)") {
		t.Fatal("startShard done channel should be buffered with shard.maxBuffered")
	}
	if strings.Contains(text, "done := make(chan string, workerCount)") {
		t.Fatal("startShard done channel still uses workerCount")
	}
}
