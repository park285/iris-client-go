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

type permanentPingError struct {
	err error
}

func (e *permanentPingError) Error() string {
	return e.err.Error()
}

func (e *permanentPingError) Unwrap() error {
	return e.err
}

// Ping method on H2CClient - implements AdminClient.Ping.
func (c *H2CClient) Ping(ctx context.Context) bool {
	return retryPing(ctx, c.logger, c.baseURL, c.pingOnce)
}

// pingOnce tries the 3-stage probe policy.
func (c *H2CClient) pingOnce(ctx context.Context) (bool, error) {
	probes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: PathReady},
		{method: http.MethodGet, path: PathHealth},
		{method: http.MethodOptions, path: PathReply},
	}

	for _, probe := range probes {
		result, err := c.probe(ctx, probe.method, probe.path)
		if err != nil {
			return false, fmt.Errorf("ping probe: %w", err)
		}

		if result.alive {
			return true, nil
		}

		if !result.fallbackable {
			return false, nil
		}
	}

	return false, nil
}

// probe executes a single HTTP probe.
func (c *H2CClient) probe(ctx context.Context, method, path string) (pingProbeResult, error) {
	req, err := c.newRequest(ctx, method, path, nil)
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

// retryPing retries with exponential backoff (50ms, 100ms), max 3 attempts.
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

// isReplyReachableStatus: 405, 401, 403, 400 all indicate the server is alive.
func isReplyReachableStatus(status int) bool {
	switch status {
	case http.StatusMethodNotAllowed, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest:
		return true
	default:
		return false
	}
}
