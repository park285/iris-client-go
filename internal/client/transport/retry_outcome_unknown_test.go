package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const retryOutcomeClientRequestID = "chatbotgo:log-42:reply-v1"

// stubResponse는 RoundTripper가 돌려줄 HTTP 응답의 상태와 본문이다.
type stubResponse struct {
	status int
	body   string
}

// transportThenResponses는 첫 attempt를 transport 오류(요청 도달 여부 불명)로 끝내고, 이후
// attempt에는 responses를 순서대로 돌려주는 RoundTripper다. 마지막 응답은 남은 attempt에 반복한다.
func transportThenResponses(network error, attempts *atomic.Int32, responses ...stubResponse) roundTripFunc {
	return func(*http.Request) (*http.Response, error) {
		attempt := int(attempts.Add(1))
		if attempt == 1 {
			return nil, network
		}

		stub := responses[min(attempt-2, len(responses)-1)]
		header := make(http.Header)

		if stub.status == http.StatusTooManyRequests {
			header.Set("Retry-After", "0")
		}

		return &http.Response{
			StatusCode: stub.status,
			Body:       io.NopCloser(strings.NewReader(stub.body)),
			Header:     header,
		}, nil
	}
}

// 앞선 attempt가 transport 오류로 끝나면 그 요청이 Iris에 admission됐는지 알 수 없다. 이후 attempt의
// 429는 clientRequestId 기록 조회 전(staging quota)에도 나올 수 있어 앞선 요청의 미도달을 증명하지
// 못하므로, 재시도를 소진한 결과는 결과 불명(ErrTransport)으로 남아야 한다
// (DEC-20260731-reply-outcome-unknown-fail-closed).
func TestReplyRetryKeepsUnknownOutcomeWhenLaterAttemptsAreRateLimited(t *testing.T) {
	t.Parallel()

	network := errors.New("temporary network failure")

	var attempts atomic.Int32

	rt := transportThenResponses(network, &attempts, stubResponse{status: http.StatusTooManyRequests, body: "slow down"})
	client := NewAPIClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))

	_, err := client.SendMessageAccepted(t.Context(), testRoom, "msg", WithClientRequestID(retryOutcomeClientRequestID))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want outcome-unknown transport error")
	}

	assertUnknownReplyOutcome(t, err, network)

	if errors.Is(err, ErrRateLimited) {
		t.Fatalf("error = %v, must not look like a rate-limit rejection after an outcome-unknown attempt", err)
	}

	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error = %v, want the final attempt's 429 kept in the message", err)
	}

	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3 (classification must not add retries)", attempts.Load())
	}
}

// 재시도하지 않는 HTTP 오류도 앞선 요청의 admission 여부를 알려주지 않는다. Failed처럼 읽히는
// ErrPermanent·ErrAuthFailed로 축약하지 않고 결과 불명으로 반환해야 한다.
func TestReplyRetryKeepsUnknownOutcomeWhenLaterAttemptFailsWithoutAdmissionAnswer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		resp stubResponse
	}{
		{name: "503", resp: stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"reply admission store unavailable"}`}},
		{name: "401", resp: stubResponse{status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`}},
		{name: "400", resp: stubResponse{status: http.StatusBadRequest, body: `{"message":"invalid reply request"}`}},
		{name: "409 without code", resp: stubResponse{status: http.StatusConflict, body: `{"message":"conflict"}`}},
		{name: "409 with unrelated code", resp: stubResponse{status: http.StatusConflict, body: `{"message":"conflict","code":"SOMETHING_ELSE"}`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			network := errors.New("temporary network failure")

			var attempts atomic.Int32

			rt := transportThenResponses(network, &attempts, tc.resp)
			client := NewAPIClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))

			_, err := client.SendMessageAccepted(t.Context(), testRoom, "msg", WithClientRequestID(retryOutcomeClientRequestID))
			if err == nil {
				t.Fatal("SendMessageAccepted() error = nil, want outcome-unknown transport error")
			}

			assertUnknownReplyOutcome(t, err, network)

			if errors.Is(err, ErrPermanent) || errors.Is(err, ErrAuthFailed) {
				t.Fatalf("error = %v, must not be reported as a definitive failure after an outcome-unknown attempt", err)
			}

			if attempts.Load() != 2 {
				t.Fatalf("attempts = %d, want 2 (non-retryable responses stop the loop)", attempts.Load())
			}
		})
	}
}

// Iris가 clientRequestId 기록을 조회한 뒤 돌려준 409 code는 그 id에 대한 판정이다. 앞선 attempt의
// 결과가 불명이었어도 소비자가 재발급·종결을 판단할 수 있게 원래 오류를 그대로 반환한다.
func TestReplyRetryReturnsClientRequestIDAdmissionAnswerAfterUnknownAttempt(t *testing.T) {
	t.Parallel()

	for _, code := range []string{
		HTTPErrorCodeClientRequestIDFailed,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDAlreadyExists,
	} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32

			rt := transportThenResponses(errors.New("temporary network failure"), &attempts,
				stubResponse{status: http.StatusConflict, body: `{"message":"conflict","code":"` + code + `"}`})
			client := NewAPIClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))

			_, err := client.SendMessageAccepted(t.Context(), testRoom, "msg", WithClientRequestID(retryOutcomeClientRequestID))
			if err == nil {
				t.Fatal("SendMessageAccepted() error = nil, want 409")
			}

			if got := HTTPErrorCode(err); got != code {
				t.Fatalf("HTTPErrorCode() = %q, want %q (err = %v)", got, code, err)
			}

			if errors.Is(err, ErrTransport) {
				t.Fatalf("error = %v, an Iris admission answer must not be reclassified as outcome unknown", err)
			}

			httpErr, ok := errors.AsType[*HTTPError](err)
			if !ok || httpErr.StatusCode != http.StatusConflict {
				t.Fatalf("error = %v, want *HTTPError 409", err)
			}
		})
	}
}

// backoff 대기 중 context가 끝날 때도 직전 attempt만이 아니라 앞선 결과 불명 attempt를 기준으로
// ErrTransport 분류를 유지해야 한다.
func TestReplyRetryWaitKeepsEarlierUnknownOutcome(t *testing.T) {
	t.Parallel()

	network := errors.New("temporary network failure")
	ctx, cancel := context.WithCancel(t.Context())

	var attempts atomic.Int32

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, network
		}

		cancel()

		header := make(http.Header)
		header.Set("Retry-After", "0")

		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("slow down")),
			Header:     header,
		}, nil
	})
	client := NewAPIClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))

	_, err := client.SendMessageAccepted(ctx, testRoom, "msg", WithClientRequestID(retryOutcomeClientRequestID))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want outcome-unknown transport error")
	}

	assertUnknownReplyOutcome(t, err, network)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, must still match context.Canceled", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

func assertUnknownReplyOutcome(t *testing.T, err, network error) {
	t.Helper()

	if !errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, want ErrTransport so consumers keep the admission outcome unknown", err)
	}

	if !errors.Is(err, network) {
		t.Fatalf("error = %v, want the outcome-unknown attempt's cause kept in the chain", err)
	}

	if _, ok := errors.AsType[*TransportError](err); !ok {
		t.Fatalf("error = %v, want it to unwrap to *TransportError", err)
	}

	// CHANGELOG·README가 적은 공개 판정 계약: 마지막 HTTP 오류는 체인에 두지 않으므로
	// *HTTPError와 code는 얻을 수 없고, ErrRetryable은 다른 transport 오류와 같은 규칙을 따른다.
	if httpErr, ok := errors.AsType[*HTTPError](err); ok {
		t.Fatalf("error = %v, want no *HTTPError in the chain, got status %d", err, httpErr.StatusCode)
	}

	if code := HTTPErrorCode(err); code != "" {
		t.Fatalf("HTTPErrorCode() = %q, want empty for an outcome-unknown error", code)
	}

	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("error = %v, want ErrRetryable like other transport errors", err)
	}
}
