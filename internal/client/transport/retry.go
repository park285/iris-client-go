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

type requestBuilder func(ctx context.Context) (*http.Request, error)

func (c *APIClient) retryPostJSON[T any](ctx context.Context, path string, hasIdempotencyKey bool, buildRequest requestBuilder) (*T, error) {
	var result *T

	err := c.retryPost(ctx, path, hasIdempotencyKey, buildRequest, func(req *http.Request) error {
		decoded, err := c.doSignedJSON[T](req, path)
		if err != nil {
			return err
		}

		result = decoded

		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
	}

	return result, nil
}

func (c *APIClient) retryPostDiscard(ctx context.Context, path string, hasIdempotencyKey bool, buildRequest requestBuilder) error {
	return c.retryPost(ctx, path, hasIdempotencyKey, buildRequest, func(req *http.Request) error { //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
		return c.doSignedDiscard(req, path)
	})
}

// 요청 생성 실패는 재시도하지 않고, 전송 이전에 initErr를 확인하는 기존 순서를 유지한다.
func (c *APIClient) retryPost(
	ctx context.Context,
	path string,
	hasIdempotencyKey bool,
	buildRequest requestBuilder,
	send func(req *http.Request) error,
) error {
	maxAttempts := 1

	if c.opts.ReplyRetryMax > 0 && path == PathReply {
		maxAttempts = c.opts.ReplyRetryMax
	}

	backoff := 50 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := buildRequest(ctx)
		if err != nil {
			return err //nolint:wrapcheck // 요청 생성기가 post 경로 맥락으로 이미 래핑한다.
		}

		if c.initErr != nil {
			return &TransportError{Op: opInit, URL: path, Err: c.initErr}
		}

		err = send(req)
		if err == nil {
			return nil
		}

		if !isRetryableReplyError(err, hasIdempotencyKey) || attempt == maxAttempts {
			return err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
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

			return retryWaitError(ctx.Err(), err, path) //nolint:wrapcheck // retryWaitError가 transport 오류로 감싼 결과를 그대로 돌려준다.
		case <-timer.C:
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return retryWaitError(ctxErr, err, path) //nolint:wrapcheck // retryWaitError가 transport 오류로 감싼 결과를 그대로 돌려준다.
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
