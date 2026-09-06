package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type responseClosingBody struct {
	io.Reader

	closes  atomic.Int32
	release <-chan struct{}
	err     error
}

func (b *responseClosingBody) Close() error {
	b.closes.Add(1)

	if b.release != nil {
		<-b.release
	}

	return b.err
}

func TestPublicDecodedResponseClosesOnce(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, payload := range []string{`{"requestId":"r1"}`, `{`, `{} {}`} {
			t.Run(method+payload, func(t *testing.T) {
				body := &responseClosingBody{Reader: strings.NewReader(payload)}
				client := NewAPIClient("https://iris.test", "synthetic-test-token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				})))

				var err error

				if method == http.MethodGet {
					_, err = client.GetReplyStatus(t.Context(), "r1")
				} else {
					_, err = client.SendMessageAccepted(t.Context(), "test-room", "test")
				}

				if payload == `{"requestId":"r1"}` && err != nil {
					t.Fatal(err)
				}

				if payload != `{"requestId":"r1"}` && err == nil {
					t.Fatal("malformed response accepted")
				}

				if calls := body.closes.Load(); calls != 1 {
					t.Fatalf("body Close calls = %d, want one owner", calls)
				}
			})
		}
	}
}

func TestPublicResponseHonorsCloseBudget(t *testing.T) {
	release := make(chan struct{})

	var releaseOnce sync.Once

	unblock := func() { releaseOnce.Do(func() { close(release) }) }

	defer unblock()

	body := &responseClosingBody{Reader: strings.NewReader(`{"requestId":"r1"}`), release: release}
	client := NewAPIClient("https://iris.test", "synthetic-test-token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})))
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)

	defer cancel()

	done := make(chan error, 1)

	go func() {
		_, err := client.SendMessageAccepted(ctx, "test-room", "test")
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, ErrTransport) || !errors.Is(err, errResponseCloseTimeout) {
			t.Fatalf("unconfirmed close = %v, want transport/close timeout", err)
		}
	case <-time.After(400 * time.Millisecond):
		t.Errorf("public call still blocked after context and 100ms close budget; Close calls = %d", body.closes.Load())
		unblock()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("call did not finish after fixture release")
		}
	}
}

func TestPublicResponseCloseFailurePreservesUnknown(t *testing.T) {
	want := errors.New("response close failed")
	body := &responseClosingBody{Reader: strings.NewReader(`{"requestId":"r1"}`), err: want}
	client := NewAPIClient("https://iris.test", "synthetic-test-token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})))
	response, err := client.SendMessageAccepted(t.Context(), "test-room", "test")

	if response != nil || !errors.Is(err, ErrTransport) || !errors.Is(err, want) {
		t.Fatalf("response=%+v error=%v, want unknown transport error", response, err)
	}

	if calls := body.closes.Load(); calls != 1 {
		t.Fatalf("body Close calls = %d, want one owner", calls)
	}
}
