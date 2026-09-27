package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

func isRetryableError(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

func isRetryableTransportError(err error) bool {
	return errors.Is(err, ErrTransport) && !errors.Is(err, ErrH3EgressDenied) && !errors.Is(err, ErrResponseTooLarge)
}

func isRetryableReplyError(err error, hasIdempotencyKey bool) bool {
	return !errors.Is(err, ErrResponseTooLarge) && (isRetryableError(err) || hasIdempotencyKey && isRetryableTransportError(err))
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
		return nil, err
	}

	return result, nil
}

func (c *APIClient) retryPostDiscard(ctx context.Context, path string, hasIdempotencyKey bool, buildRequest requestBuilder) error {
	return c.retryPost(ctx, path, hasIdempotencyKey, buildRequest, func(req *http.Request) error {
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

	// unknownErr는 결과를 알 수 없는(요청이 Iris에 도달해 admission됐을 수 있는) 가장 최근 attempt의
	// transport 오류다. 이후 attempt가 이 결과를 판정하지 못하고 끝나면 반환 오류가 이 분류를 유지한다.
	var unknownErr error

	// maxAttempts는 항상 1 이상이고 마지막 attempt는 아래 attempt == maxAttempts 판정에서
	// 반드시 반환한다. 루프 종료는 그 판정 하나가 소유하므로 루프 조건과 루프 뒤 반환을 두지 않는다.
	for attempt := 1; ; attempt++ {
		req, err := buildRequest(ctx)
		if err != nil {
			return err
		}

		if c.initErr != nil {
			return &TransportError{Op: opInit, URL: path, Err: c.initErr}
		}

		err = send(req)
		if err == nil {
			return nil
		}

		if errors.Is(err, ErrTransport) {
			unknownErr = err
		}

		if !isRetryableReplyError(err, hasIdempotencyKey) || attempt == maxAttempts {
			return keepUnknownOutcome(err, unknownErr, path)
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

			return retryWaitError(ctx.Err(), unknownErr, path)
		case <-timer.C:
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return retryWaitError(ctxErr, unknownErr, path)
		}

		backoff = min(backoff*2, time.Second)
	}
}

// 앞선 attempt 중 transport 오류로 끝난 것이 있으면 요청 도달 여부를 알 수 없으므로, 대기 중
// 만료된 context 오류도 ErrTransport 계열로 감싸 소비자의 admission-lost 판정을 유지한다.
func retryWaitError(waitErr, unknownErr error, path string) error {
	if unknownErr == nil {
		return waitErr
	}

	return &TransportError{
		Op:  opRetryWait,
		URL: path,
		Err: fmt.Errorf("%w (outcome-unknown attempt: %w)", waitErr, unknownErr),
	}
}

// keepUnknownOutcome은 앞선 attempt의 결과가 불명이면 마지막 attempt 오류가 그 결과를 판정하지
// 못하는 한 ErrTransport로 반환한다(DEC-20260731-reply-outcome-unknown-fail-closed). 429·5xx·
// 그 밖의 4xx는 Iris가 clientRequestId 기록을 조회하기 전에도 나오므로 앞선 요청이 admission되지
// 않았음을 증명하지 못한다. 마지막 오류를 체인에 두면 ErrPermanent·ErrRateLimited가 먼저 매칭돼
// 결과 불명이 Failed·미처리로 축약되므로 마지막 오류는 메시지에만 남긴다.
func keepUnknownOutcome(finalErr, unknownErr error, path string) error {
	if unknownErr == nil || errors.Is(finalErr, ErrTransport) || isClientRequestIDAdmissionAnswer(finalErr) {
		return finalErr
	}

	return &TransportError{
		Op:  opRetryOutcomeUnknown,
		URL: path,
		Err: fmt.Errorf("%w (final attempt: %s)", unknownErr, finalErr.Error()),
	}
}

// Iris가 clientRequestId 기록을 조회한 뒤 돌려준 409 code만 그 id의 admission 판정이다.
func isClientRequestIDAdmissionAnswer(err error) bool {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr == nil || httpErr.StatusCode != http.StatusConflict {
		return false
	}

	return slices.Contains([]string{
		HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDFailed,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDAlreadyExists,
	}, HTTPErrorCode(err))
}
