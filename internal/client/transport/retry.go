package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func isRetryableError(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

func isRetryableTransportError(err error) bool {
	return errors.Is(err, ErrTransport) && !errors.Is(err, ErrH3EgressDenied)
}

func isRetryableReplyError(err error, hasIdempotencyKey bool) bool {
	return isRetryableError(err) || hasIdempotencyKey && isRetryableTransportError(err)
}

func (c *H2CClient) postWithRetry(
	ctx context.Context,
	path string,
	hasIdempotencyKey bool,
	buildRequest func(context.Context) (*http.Request, error),
	out any,
) error {
	maxAttempts := 1
	if c.opts.ReplyRetryMax > 0 && path == PathReply {
		maxAttempts = c.opts.ReplyRetryMax
	}

	backoff := 50 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := buildRequest(ctx)
		if err != nil {
			return err
		}

		err = c.doSignedJSON(req, path, out)
		if err == nil {
			return nil
		}

		if !isRetryableReplyError(err, hasIdempotencyKey) || attempt == maxAttempts {
			return err
		}

		delay, retryAfterApplied := retryDelayAndRetryAfter(err, backoff)
		c.opts.TransportMetrics.ObserveReplyRetry(attempt, delay)
		if retryAfterApplied {
			c.opts.TransportMetrics.ObserveReplyRetryAfter(delay)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return retryWaitError(ctx.Err(), err, path)
		case <-timer.C:
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return retryWaitError(ctxErr, err, path)
		}

		backoff = min(backoff*2, time.Second)
	}

	return fmt.Errorf("post %s: retries exhausted", path)
}

// 직전 attempt가 transport 오류였다면 요청 도달 여부를 알 수 없으므로, 대기 중 만료된
// context 오류도 ErrTransport 계열로 감싸 소비자의 admission-lost 판정을 유지한다.
func retryWaitError(waitErr, attemptErr error, path string) error {
	if !errors.Is(attemptErr, ErrTransport) {
		return waitErr
	}

	return &TransportError{
		Op:  opRetryWait,
		URL: path,
		Err: fmt.Errorf("%w (last attempt: %w)", waitErr, attemptErr),
	}
}
