package webhook

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestInternalPool_SubmitWait_Success(t *testing.T) {
	t.Parallel()

	pool := newInternalPool(1, 1)
	defer pool.StopAndWait()

	done := make(chan struct{})
	if ok := pool.SubmitWait(func() {
		close(done)
	}); !ok {
		t.Fatal("SubmitWait() = false, want true")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task did not run")
	}
}

func TestInternalPool_SubmitWait_StopUnblocks(t *testing.T) {
	t.Parallel()

	pool := newInternalPool(0, 0)
	started := make(chan struct{})
	result := make(chan bool, 1)

	go func() {
		close(started)
		result <- pool.SubmitWait(func() {})
	}()

	<-started

	select {
	case got := <-result:
		t.Fatalf("SubmitWait() returned early: %v", got)
	case <-time.After(20 * time.Millisecond):
	}

	pool.StopAndWait()

	select {
	case got := <-result:
		if got {
			t.Fatal("SubmitWait() = true after StopAndWait, want false")
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitWait() did not unblock after StopAndWait")
	}
}

func TestInternalPool_StopAndWait_Drains(t *testing.T) {
	t.Parallel()

	pool := newInternalPool(1, 2)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var completed atomic.Int32

	if ok := pool.SubmitWait(func() {
		started <- struct{}{}
		<-release
		completed.Add(1)
	}); !ok {
		t.Fatal("first SubmitWait() = false, want true")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}

	if ok := pool.SubmitWait(func() {
		completed.Add(1)
	}); !ok {
		t.Fatal("second SubmitWait() = false, want true")
	}

	stopped := make(chan struct{})
	go func() {
		pool.StopAndWait()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("StopAndWait returned before queued work drained")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopAndWait did not return after work drained")
	}

	if got := completed.Load(); got != 2 {
		t.Fatalf("completed tasks = %d, want 2", got)
	}
}

func internalPoolClosed(pool *internalPool) bool {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.closed
}
