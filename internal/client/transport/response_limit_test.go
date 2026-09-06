package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const responseLimitContract = 16 << 20

const (
	memberResponsePrefix = `{"chatId":1,"members":[{"userId":7,"nickname":"`
	memberResponseSuffix = `"}],"totalCount":1}`
)

type responseLimitBody struct {
	io.Reader

	bytesRead atomic.Int64
	closes    atomic.Int32
}

func (b *responseLimitBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.bytesRead.Add(int64(n))

	return n, err
}

func (b *responseLimitBody) Close() error {
	b.closes.Add(1)

	return nil
}

func TestTypedResponseRejectsOversizedFirstJSONValue(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			body := &responseLimitBody{Reader: strings.NewReader(`{"detail":"` + strings.Repeat("x", 2*responseLimitContract) + `"}`)}

			var attempts atomic.Int32

			client := NewAPIClient("https://iris.test", "synthetic-test-token", WithReplyRetry(3), WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts.Add(1)

				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})))

			var err error

			if method == http.MethodGet {
				_, err = client.GetReplyStatus(t.Context(), "test")
			} else {
				_, err = client.SendMessageAccepted(t.Context(), "test-room", "test", WithClientRequestID("limit-test"))
			}

			if !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("oversized response error = %v, want ErrResponseTooLarge", err)
			}

			if method == http.MethodPost && !errors.Is(err, ErrTransport) {
				t.Fatalf("POST must preserve outcome unknown: %v", err)
			}

			if errors.Is(err, ErrRetryable) {
				t.Fatal("deterministic byte limit must not be retryable")
			}

			if attempts.Load() != 1 {
				t.Fatalf("attempts = %d, want no automatic retry", attempts.Load())
			}

			if body.closes.Load() != 1 {
				t.Fatalf("Close calls = %d", body.closes.Load())
			}

			if body.bytesRead.Load() > responseLimitContract+1 {
				t.Fatalf("read %d bytes past total budget", body.bytesRead.Load())
			}
		})
	}
}

func memberResponseWithBytes(size int) string {
	const unit = "가🙂"

	remaining := size - len(memberResponsePrefix) - len(memberResponseSuffix)

	return memberResponsePrefix + strings.Repeat(unit, remaining/len(unit)) + strings.Repeat("x", remaining%len(unit)) + memberResponseSuffix
}

func TestTypedResponseTotalByteBoundaryPreservesUnicodeAndTrailer(t *testing.T) {
	for _, tc := range []struct {
		name          string
		size, trailer int
		tooLarge      bool
	}{
		{"below", responseLimitContract - 1, 0, false},
		{"exact", responseLimitContract, 0, false},
		{"above", responseLimitContract + 1, 0, true},
		{"exact_with_trailer", responseLimitContract - 1, 1, false},
		{"above_in_trailer", responseLimitContract, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 16 MiB 경계에서 JSON이 끝난 뒤 별도 Read로 trailer를 읽도록 나눈다.
			body := &responseLimitBody{Reader: io.MultiReader(strings.NewReader(memberResponseWithBytes(tc.size)), strings.NewReader(strings.Repeat(" ", tc.trailer)))}
			client := NewAPIClient("https://iris.test", "synthetic-test-token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})))
			response, err := client.GetMembers(t.Context(), 1)

			if body.bytesRead.Load() != int64(min(tc.size+tc.trailer, responseLimitContract+1)) {
				t.Fatalf("read bytes=%d, payload=%d", body.bytesRead.Load(), tc.size+tc.trailer)
			}

			if body.closes.Load() != 1 {
				t.Fatalf("Close calls=%d", body.closes.Load())
			}

			if tc.tooLarge {
				if response != nil || !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("response present=%t error=%v", response != nil, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if response == nil || len(response.Members) != 1 || response.Members[0].Nickname == nil || !strings.HasPrefix(*response.Members[0].Nickname, "가🙂") {
				t.Fatal("supported member response was lost or truncated")
			}

			if len(*response.Members[0].Nickname) != tc.size-len(memberResponsePrefix)-len(memberResponseSuffix) {
				t.Fatal("member nickname was truncated")
			}
		})
	}
}

func TestTypedResponseBoundsOversizedUnknownArray(t *testing.T) {
	body := &responseLimitBody{Reader: strings.NewReader(`{"detail":null,"unknown":[` + strings.Repeat("0,", responseLimitContract/2) + `0]}`)}
	client := NewAPIClient("https://iris.test", "synthetic-test-token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})))
	response, err := client.GetReplyStatus(t.Context(), "test")

	if response != nil || !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("array response present=%t error=%v", response != nil, err)
	}

	if got := body.bytesRead.Load(); got > responseLimitContract+1 {
		t.Fatalf("read %d bytes past array budget", got)
	}
}
