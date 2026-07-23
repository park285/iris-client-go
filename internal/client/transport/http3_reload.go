package transport

import (
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// reloadingH3Transport는 CA 변경 시 transport를 원자 교체하고 실패 시 기존 값을 유지한다.
type reloadingH3Transport struct {
	current  atomic.Pointer[http3.Transport]
	opts     clientOptions
	caFile   string
	interval time.Duration
	grace    time.Duration
	logger   *slog.Logger

	lastHash  [sha256.Size]byte
	stop      chan struct{}
	watchDone chan struct{}
	stale     sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

var (
	_ http.RoundTripper = (*reloadingH3Transport)(nil)
	_ io.Closer         = (*reloadingH3Transport)(nil)
)

// newReloadingH3Transport는 initialPEM(초기 transport를 만든 그 바이트)으로 기준 해시를 시드한다.
// 호출자(selectTransport)가 CA를 한 번만 읽어 transport와 해시를 동일 바이트에서 만들도록 바이트를 넘긴다.
func newReloadingH3Transport(initial *http3.Transport, opts clientOptions, caFile string, interval time.Duration, initialPEM []byte) *reloadingH3Transport {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	grace := opts.Timeout

	r := &reloadingH3Transport{
		opts:      opts,
		caFile:    caFile,
		interval:  interval,
		grace:     grace,
		logger:    logger,
		lastHash:  sha256.Sum256(initialPEM),
		stop:      make(chan struct{}),
		watchDone: make(chan struct{}),
	}
	r.current.Store(initial)

	safeGo(r.logger, "iris_h3_ca_watch_panic_recovered", r.watch)

	return r
}

func (r *reloadingH3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.current.Load().RoundTrip(req)
}

func (r *reloadingH3Transport) watch() {
	defer close(r.watchDone)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.reloadIfChanged()
		}
	}
}

func (r *reloadingH3Transport) reloadIfChanged() {
	data, err := os.ReadFile(r.caFile)
	if err != nil {
		r.logger.Warn("iris_h3_ca_reload_read_failed", slog.String("file", r.caFile), slog.Any("error", err))
		return
	}

	sum := sha256.Sum256(data)
	if sum == r.lastHash {
		return
	}

	next, err := newHTTP3TransportFromCA(r.opts, true, data)
	if err != nil {
		r.logger.Warn("iris_h3_ca_reload_build_failed", slog.String("file", r.caFile), slog.Any("error", err))
		return
	}

	r.lastHash = sum
	old := r.current.Swap(next)
	r.scheduleStaleClose(old)
	r.logger.Info("iris_h3_ca_reloaded", slog.String("file", r.caFile))
}

func (r *reloadingH3Transport) scheduleStaleClose(old *http3.Transport) {
	if old == nil {
		return
	}

	r.stale.Add(1)
	safeGo(r.logger, "iris_h3_stale_close_panic_recovered", func() {
		defer r.stale.Done()
		if r.grace > 0 {
			timer := time.NewTimer(r.grace)
			defer timer.Stop()

			select {
			case <-timer.C:
			case <-r.stop:
			}
		}

		if err := old.Close(); err != nil {
			r.logger.Warn("iris_h3_stale_transport_close_failed", slog.Any("error", err))
		}
	})
}

func (r *reloadingH3Transport) Close() error {
	r.closeOnce.Do(func() {
		close(r.stop)
		<-r.watchDone
		r.stale.Wait()
		if cur := r.current.Load(); cur != nil {
			r.closeErr = cur.Close()
		}
	})
	return r.closeErr
}
