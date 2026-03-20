package webhook

import "sync"

type scheduledTask struct {
	task webhookTask
	key  string
}

type taskRunner func(index int, task webhookTask)

type scheduler struct {
	incoming    chan webhookTask
	maxBuffered int
	wg          sync.WaitGroup
}

func newScheduler(queueSize int) *scheduler {
	return &scheduler{
		incoming:    make(chan webhookTask, queueSize),
		maxBuffered: queueSize,
	}
}

func (s *scheduler) start(workerCount int, runner taskRunner) {
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
		}(i)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(work)
		runDispatcher(s.incoming, work, done, s.maxBuffered)
	}()
}

func (s *scheduler) close() {
	close(s.incoming)
	s.wg.Wait()
}

func runDispatcher(incoming <-chan webhookTask, work chan<- scheduledTask, done <-chan string, maxBuffered int) {
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

		case workCh <- next:
			ready = ready[1:]
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
