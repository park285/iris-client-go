package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type pingProbeResult struct {
	alive        bool
	fallbackable bool
}

type cachedPingProbe struct {
	method string
	path   string
}

type permanentPingError struct {
	err error
}

func (e *permanentPingError) Error() string {
	return e.err.Error()
}

func (e *permanentPingError) Unwrap() error {
	return e.err
}

// Ping은 H2CClient의 핑 메서드로, AdminClient.Ping을 구현합니다.
func (c *H2CClient) Ping(ctx context.Context) bool {
	return retryPing(ctx, c.logger, c.baseURL, c.pingOnce)
}

// pingOnce는 설정된 프로브 정책을 시도합니다.
func (c *H2CClient) pingOnce(ctx context.Context) (bool, error) {
	probes := c.resolveProbes()
	for _, probe := range probes {
		result, err := c.probe(ctx, probe.method, probe.path)
		if err != nil {
			return false, fmt.Errorf("ping probe: %w", err)
		}

		if result.alive {
			c.cachedProbe.Store(&cachedPingProbe{method: probe.method, path: probe.path})
			return true, nil
		}

		if !result.fallbackable {
			return false, nil
		}
	}

	return false, nil
}

func (c *H2CClient) resolveProbes() []struct{ method, path string } {
	if cached, ok := c.cachedProbe.Load().(*cachedPingProbe); ok && cached != nil {
		return []struct{ method, path string }{{cached.method, cached.path}}
	}

	switch c.opts.PingStrategy {
	case PingStrategyReady:
		return []struct{ method, path string }{{http.MethodGet, PathReady}}
	case PingStrategyHealth:
		return []struct{ method, path string }{{http.MethodGet, PathHealth}}
	default:
		return []struct{ method, path string }{
			{http.MethodGet, PathReady},
			{http.MethodGet, PathHealth},
			{http.MethodOptions, PathReply},
		}
	}
}

// probe는 단일 HTTP 프로브를 실행합니다.
func (c *H2CClient) probe(ctx context.Context, method, path string) (pingProbeResult, error) {
	probeCtx := ctx
	if c.opts.PingProbeTimeout > 0 {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithTimeout(ctx, c.opts.PingProbeTimeout)
		defer cancel()
	}

	req, err := c.newRequest(probeCtx, method, path, nil, SecretRoleBotControl)
	if err != nil {
		return pingProbeResult{}, fmt.Errorf("build probe request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return pingProbeResult{}, fmt.Errorf("probe %s %s: %w", method, path, err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // Best-effort drain for keep-alive reuse.
		resp.Body.Close()              //nolint:errcheck,gosec // Best-effort body close.
	}()

	result, classifyErr := classifyProbeResult(method, path, resp.StatusCode)
	if classifyErr != nil {
		return pingProbeResult{}, fmt.Errorf("classify probe result: %w", classifyErr)
	}

	return result, nil
}

func classifyProbeResult(method, path string, statusCode int) (pingProbeResult, error) {
	switch path {
	case PathReady, PathHealth:
		result, err := classifyHealthProbeResult(method, path, statusCode)
		if err != nil {
			return pingProbeResult{}, fmt.Errorf("classify health probe: %w", err)
		}

		return result, nil
	case PathReply:
		result, err := classifyReplyProbeResult(method, path, statusCode)
		if err != nil {
			return pingProbeResult{}, fmt.Errorf("classify reply probe: %w", err)
		}

		return result, nil
	default:
		result, err := classifyDefaultProbeResult(method, path, statusCode)
		if err != nil {
			return pingProbeResult{}, fmt.Errorf("classify default probe: %w", err)
		}

		return result, nil
	}
}

func classifyHealthProbeResult(method, path string, statusCode int) (pingProbeResult, error) {
	switch statusCode {
	case http.StatusOK:
		return pingProbeResult{alive: true}, nil
	case http.StatusNotFound:
		return pingProbeResult{fallbackable: true}, nil
	default:
		err := probeStatusError(method, path, statusCode)
		return pingProbeResult{}, fmt.Errorf("health probe status: %w", err)
	}
}

func classifyReplyProbeResult(method, path string, statusCode int) (pingProbeResult, error) {
	if isReplyReachableStatus(statusCode) {
		return pingProbeResult{alive: true}, nil
	}

	err := probeStatusError(method, path, statusCode)

	return pingProbeResult{}, fmt.Errorf("reply probe status: %w", err)
}

func classifyDefaultProbeResult(method, path string, statusCode int) (pingProbeResult, error) {
	if statusCode >= 200 && statusCode < 400 {
		return pingProbeResult{alive: true}, nil
	}

	err := probeStatusError(method, path, statusCode)

	return pingProbeResult{}, fmt.Errorf("default probe status: %w", err)
}

func probeStatusError(method, path string, statusCode int) error {
	err := fmt.Errorf("probe %s %s returned %d", method, path, statusCode)
	if statusCode >= 400 && statusCode < 500 {
		return &permanentPingError{err: err}
	}

	return err
}

// retryPing은 지수 백오프(50ms, 100ms)로 최대 3회 재시도합니다.
func retryPing(ctx context.Context, logger *slog.Logger, baseURL string, fn func(context.Context) (bool, error)) bool {
	backoff := 50 * time.Millisecond
	maxBackoff := 100 * time.Millisecond

	for attempt := 1; attempt <= 3; attempt++ {
		alive, err := fn(ctx)
		if err == nil {
			return alive
		}

		if shouldStopRetry(logger, baseURL, attempt, err) {
			return false
		}

		if !waitRetryDelay(ctx, backoff) {
			return false
		}

		backoff = nextBackoff(backoff, maxBackoff)
	}

	return false
}

func shouldStopRetry(logger *slog.Logger, baseURL string, attempt int, err error) bool {
	var permanent *permanentPingError
	if errors.As(err, &permanent) {
		logPingPermanentFailure(logger, baseURL, attempt, permanent)
		return true
	}

	logPingRetry(logger, baseURL, attempt, err)

	return attempt == 3
}

func logPingPermanentFailure(logger *slog.Logger, baseURL string, attempt int, err *permanentPingError) {
	if logger == nil {
		return
	}

	logger.Warn("iris_ping_permanent_failure", "base_url", baseURL, "attempt", attempt, "error", err.Error())
}

func logPingRetry(logger *slog.Logger, baseURL string, attempt int, err error) {
	if logger == nil {
		return
	}

	logger.Warn("iris_ping_retry", "base_url", baseURL, "attempt", attempt, "error", err)
}

func waitRetryDelay(ctx context.Context, backoff time.Duration) bool {
	timer := time.NewTimer(backoff)

	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(backoff, maxBackoff time.Duration) time.Duration {
	backoff *= 2
	if backoff > maxBackoff {
		return maxBackoff
	}

	return backoff
}

// isReplyReachableStatus는 405, 401, 403, 400 모두 서버가 살아있음을 나타내는지 판별합니다.
func isReplyReachableStatus(status int) bool {
	switch status {
	case http.StatusMethodNotAllowed, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest:
		return true
	default:
		return false
	}
}
