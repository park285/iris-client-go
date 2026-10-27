package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type responseClosingBody struct {
	io.Reader

	closes atomic.Int32
	err    error
}

func (b *responseClosingBody) Close() error {
	b.closes.Add(1)

	return b.err
}

func TestPublicDecodedResponseClosesOnce(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, payload := range []string{`{"requestId":"r1"}`, `{`, `{} {}`} {
			t.Run(method+payload, func(t *testing.T) {
				body := &responseClosingBody{Reader: strings.NewReader(payload)}
				client := mustNewAPIClient("https://iris.test", "synthetic-test-token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
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

func TestPublicResponseCloseFailurePreservesUnknown(t *testing.T) {
	want := errors.New("response close failed")
	body := &responseClosingBody{Reader: strings.NewReader(`{"requestId":"r1"}`), err: want}
	client := mustNewAPIClient("https://iris.test", "synthetic-test-token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
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
