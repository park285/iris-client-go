package webhook

import "sync"

type internalPool struct {
	queue    chan func()
	stopCh   chan struct{}
	mu       sync.RWMutex
	closed   bool
	workerWG sync.WaitGroup
	stopOnce sync.Once
}

func newInternalPool(workers, queueSize int) *internalPool {
	if workers < 0 {
		workers = 0
	}
	if queueSize < 0 {
		queueSize = 0
	}

	pool := &internalPool{
		queue:  make(chan func(), queueSize),
		stopCh: make(chan struct{}),
	}

	for range workers {
		pool.workerWG.Add(1)
		// crosscutting:allow internalPool은 runScheduledTask로 panic 격리된 scheduler callback만 실행한다.
		go pool.worker()
	}

	return pool
}

func (p *internalPool) SubmitWait(task func()) bool {
	if p == nil || task == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return false
	}

	select {
	case p.queue <- task:
		return true
	case <-p.stopCh:
		return false
	}
}

func (p *internalPool) StopAndWait() {
	if p == nil {
		return
	}

	p.stopOnce.Do(func() {
		close(p.stopCh)

		p.mu.Lock()
		p.closed = true
		close(p.queue)
		p.mu.Unlock()
	})

	p.workerWG.Wait()
}

func (p *internalPool) worker() {
	defer p.workerWG.Done()

	for task := range p.queue {
		if task == nil {
			continue
		}

		task()
	}
}
