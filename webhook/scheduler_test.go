package webhook

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerPreservesPerKeyOrder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen []string

	sched := newScheduler(100)
	sched.start(2, func(_ int, task webhookTask) {
		time.Sleep(time.Millisecond)
		mu.Lock()
		seen = append(seen, task.msg.Msg)
		mu.Unlock()
	})

	threadA := "a"
	for i := range 5 {
		msg := &Message{Room: "room", Msg: fmt.Sprintf("a-%d", i), JSON: &MessageJSON{ThreadID: &threadA}}
		sched.incoming <- webhookTask{msg: msg}
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

	sched := newScheduler(100)
	sched.start(4, func(_ int, _ webhookTask) {
		processed.Add(1)
	})

	threadA := "a"
	threadB := "b"
	for range 10 {
		sched.incoming <- webhookTask{msg: &Message{Room: "r", Msg: "a", JSON: &MessageJSON{ThreadID: &threadA}}}
		sched.incoming <- webhookTask{msg: &Message{Room: "r", Msg: "b", JSON: &MessageJSON{ThreadID: &threadB}}}
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

	sched := newScheduler(10)
	sched.start(1, func(_ int, _ webhookTask) {
		processed.Add(1)
		<-block
	})

	sched.incoming <- webhookTask{msg: &Message{Msg: "first"}}
	sched.incoming <- webhookTask{msg: &Message{Msg: "second"}}

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
	sched := newScheduler(queueSize)
	sched.start(1, func(_ int, _ webhookTask) {
		received.Add(1)
		<-block
	})

	threadA := "a"

	// 1번째: worker에서 처리 중 (block)
	sched.incoming <- webhookTask{msg: &Message{Room: "r", Msg: "1", JSON: &MessageJSON{ThreadID: &threadA}}}
	time.Sleep(10 * time.Millisecond)

	// 2~4번째: dispatcher pending에 적재 (buffered = queueSize = 3)
	for i := range queueSize {
		sched.incoming <- webhookTask{msg: &Message{Room: "r", Msg: fmt.Sprintf("%d", i+2), JSON: &MessageJSON{ThreadID: &threadA}}}
	}

	// dispatcher 내부 buffered가 maxBuffered에 도달하여 incoming 읽기를 중단해야 함
	// 추가 전송 시도는 즉시 실패해야 함
	select {
	case sched.incoming <- webhookTask{msg: &Message{Room: "r", Msg: "overflow"}}:
		t.Fatal("send should block when capacity is reached")
	case <-time.After(50 * time.Millisecond):
	}

	close(block)
	sched.close()
}
