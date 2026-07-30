package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostWithRetryContextDeadlineDuringBackoffStaysTransportError(t *testing.T) {
	t.Parallel()

	network := errors.New("temporary network failure")
	var attempts atomic.Int32
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)

		return nil, network
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()

	_, err := client.SendMessageAccepted(ctx, "room", "msg", WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want a transport error")
	}
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, must stay ErrTransport so consumers treat admission as lost", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, must still match context.DeadlineExceeded", err)
	}
	if !errors.Is(err, network) {
		t.Fatalf("error = %v, must keep the last transport failure", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 before the deadline expired", attempts.Load())
	}

	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("error = %v, want it to unwrap to *TransportError", err)
	}

	// 같은 transport 실패가 ctx 만료 없이 반환될 때도 ErrRetryable이므로, 대기 중 만료가
	// 겹쳤다는 타이밍 우연만으로 분류가 갈리지 않게 고정한다.
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("error = %v, want ErrRetryable to stay consistent with a bare transport failure", err)
	}
}

func TestPostWithRetryContextCancelDuringBackoffStaysTransportError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()

		return nil, errors.New("temporary network failure")
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))

	_, err := client.SendMessageAccepted(ctx, "room", "msg", WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, want ErrTransport", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, must still match context.Canceled", err)
	}
}

func TestPostWithRetryContextDeadlineAfterHTTPErrorStaysBareContextError(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("slow down")),
			Header:     make(http.Header),
		}, nil
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt), WithReplyRetry(3))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()

	_, err := client.SendMessageAccepted(ctx, "room", "msg", WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, must not be reclassified as a transport error after an HTTP status failure", err)
	}
}
