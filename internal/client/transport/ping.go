package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const pingDrainMaxBytes = 4 << 10

func (c *APIClient) Ping(ctx context.Context) bool {
	return retryPing(ctx, c.logger, c.baseURL, c.probe)
}

// probe는 GET /ready 하나만 확인한다. 다른 endpoint로 넘어가는 경로가 없으므로 200 외의
// 응답은 모두 실패다: 4xx(404 포함)는 설정 오류라 재시도하지 않는 *PingError, 나머지는
// retryPing이 재시도하는 일시 오류로 보고한다. PingProbeTimeout은 시도 하나의 상한이며
// applyClientOptions가 항상 양수(기본 5초)로 채운다.
func (c *APIClient) probe(ctx context.Context) error {
	const path = PathReady

	probeCtx, cancel := context.WithTimeout(ctx, c.opts.PingProbeTimeout)
	defer cancel()

	req, err := c.newSignedRequest(probeCtx, http.MethodGet, path, nil, SecretRoleBotControl)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &TransportError{Op: "ping", URL: redactedURLForError(req.URL.String()), Err: err}
	}

	defer resp.Body.Close()

	defer drainBounded(resp.Body, pingDrainMaxBytes)

	if resp.StatusCode != http.StatusOK {
		return probeStatusError(path, resp.StatusCode)
	}

	return nil
}

func probeStatusError(path string, statusCode int) error {
	err := fmt.Errorf("probe GET %s returned %d", path, statusCode)
	if statusCode >= 400 && statusCode < 500 {
		return &PingError{URL: path, Reason: err.Error(), Err: err}
	}

	return err
}

func retryPing(ctx context.Context, logger *slog.Logger, baseURL string, fn func(context.Context) error) bool {
	backoff := 50 * time.Millisecond
	maxBackoff := 100 * time.Millisecond

	for attempt := 1; attempt <= 3; attempt++ {
		err := fn(ctx)
		if err == nil {
			return true
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
	if permanent, ok := errors.AsType[*PingError](err); ok {
		logPingPermanentFailure(logger, baseURL, attempt, permanent)

		return true
	}

	logPingRetry(logger, baseURL, attempt, err)

	return attempt == 3
}

func logPingPermanentFailure(logger *slog.Logger, baseURL string, attempt int, err *PingError) {
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
