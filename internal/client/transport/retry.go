package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type stopReplyRetryError struct {
	cause error
}

func (e *stopReplyRetryError) Error() string { return e.cause.Error() }
func (e *stopReplyRetryError) Unwrap() error { return e.cause }

func isRetryableError(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

func isRetryableTransportError(err error) bool {
	return (errors.Is(err, ErrTransport) || errors.Is(err, ErrNotSent)) &&
		!errors.Is(err, ErrH3EgressDenied) && !errors.Is(err, ErrResponseTooLarge)
}

func isRetryableReplyError(err error, hasIdempotencyKey bool) bool {
	if _, stop := errors.AsType[*stopReplyRetryError](err); stop {
		return false
	}

	return !errors.Is(err, ErrResponseTooLarge) && (isRetryableError(err) || hasIdempotencyKey && isRetryableTransportError(err))
}

type requestBuilder func(ctx context.Context) (*http.Request, error)

func (c *APIClient) retryPostJSON[T any](ctx context.Context, path string, hasIdempotencyKey bool, buildRequest requestBuilder) (*T, error) {
	var result *T

	err := c.retryPost(ctx, path, hasIdempotencyKey, buildRequest, func(req *http.Request, cancel context.CancelCauseFunc, epochRefreshed *bool) error {
		decoded, err := c.doSignedJSON[T](req, cancel, path, hasIdempotencyKey, epochRefreshed)
		if err != nil {
			return err
		}

		result = decoded

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *APIClient) retryPost(
	ctx context.Context,
	path string,
	hasIdempotencyKey bool,
	buildRequest requestBuilder,
	send func(req *http.Request, cancel context.CancelCauseFunc, epochRefreshed *bool) error,
) error {
	maxAttempts := 1

	if c.opts.ReplyRetryMax > 0 && path == PathReply {
		maxAttempts = c.opts.ReplyRetryMax
	}

	backoff := 50 * time.Millisecond

	var (
		unknownErr     error
		epochRefreshed bool
	)

	for attempt := 1; ; attempt++ {
		attemptCtx, cancelAttempt := c.requestContext(ctx)

		req, err := buildRequest(attemptCtx)
		if err != nil {
			cancelAttempt(nil)

			return keepUnknownOutcome(err, unknownErr, path, hasIdempotencyKey)
		}

		err = send(req, cancelAttempt, &epochRefreshed)
		cancelAttempt(nil)

		if err == nil {
			return nil
		}

		if errors.Is(err, ErrTransport) {
			unknownErr = err
		}

		if !isRetryableReplyError(err, hasIdempotencyKey) || attempt == maxAttempts {
			return keepUnknownOutcome(err, unknownErr, path, hasIdempotencyKey)
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

			return retryWaitError(ctx.Err(), unknownErr, path, hasIdempotencyKey)
		case <-timer.C:
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return retryWaitError(ctxErr, unknownErr, path, hasIdempotencyKey)
		}

		backoff = min(backoff*2, time.Second)
	}
}

func retryWaitError(waitErr, unknownErr error, path string, idempotent bool) error {
	if unknownErr == nil {
		return waitErr
	}

	return &TransportError{
		Op:         opRetryWait,
		URL:        path,
		Err:        fmt.Errorf("%w (outcome-unknown attempt: %w)", waitErr, unknownErr),
		Idempotent: idempotent,
	}
}

func keepUnknownOutcome(finalErr, unknownErr error, path string, idempotent bool) error {
	if unknownErr == nil || errors.Is(finalErr, ErrTransport) || ClientRequestIDConflictCode(finalErr) != "" {
		return finalErr
	}

	return &TransportError{
		Op:         opRetryOutcomeUnknown,
		URL:        path,
		Err:        fmt.Errorf("%w (final attempt: %s)", unknownErr, finalErr.Error()),
		Idempotent: idempotent,
	}
}
