package transport

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadErrorBodyDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 256<<10))}

	_ = readTruncatedErrorBody(reader)

	maxRead := int64(httpErrorBodyParseMaxLen + 1 + httpErrorBodyDrainMaxLen)
	if reader.bytesRead > maxRead {
		t.Fatalf("readErrorBody read %d bytes, want at most %d", reader.bytesRead, maxRead)
	}
}

func TestDoRequestDrainsTrailerAfterDecode(t *testing.T) {
	t.Parallel()

	body := `{"requestId":"req-1"}` + strings.Repeat("\n", 64)
	reader := &countingReader{Reader: strings.NewReader(body)}
	c := mustNewAPIClient("https://iris.test", "token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)}, nil
	})))

	if _, err := c.SendMessageAccepted(t.Context(), testRoom, "hello"); err != nil {
		t.Fatalf("SendMessageAccepted() error = %v", err)
	}

	if reader.bytesRead != int64(len(body)) {
		t.Fatalf("decoded response read %d of %d bytes; the trailer must be drained to reach EOF", reader.bytesRead, len(body))
	}
}

func TestDecodedResponseDrainClosesStalledBodyWithinTimeBudget(t *testing.T) {
	t.Parallel()

	var body *stalledDecodedBody

	client := mustNewAPIClient("https://iris.test", "token", withRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body = newStalledDecodedBody(req.Context(), `{"requestId":"req-1"}`)

		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})))

	started := time.Now()
	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")

	if !errors.Is(err, ErrResponseDrainTimeout) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrResponseDrainTimeout", err)
	}

	if !errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport classification", err)
	}

	if elapsed := time.Since(started); elapsed > 10*successBodyDrainTimeout {
		t.Fatalf("decoded response drain took %s", elapsed)
	}

	if !body.closed.Load() {
		t.Fatal("stalled decoded response body was not closed")
	}
}

func TestDecodedCompressedResponseDrainIsTimeBounded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)

		compressed := gzip.NewWriter(w)
		if _, err := compressed.Write([]byte(`{"requestId":"req-1"}`)); err != nil {
			t.Errorf("write compressed response: %v", err)

			return
		}

		if err := compressed.Flush(); err != nil {
			t.Errorf("flush compressed response: %v", err)

			return
		}

		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush response: %v", err)

			return
		}

		<-r.Context().Done()
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "token", WithHTTPClient(server.Client()))
	started := time.Now()

	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")
	if !errors.Is(err, ErrResponseDrainTimeout) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrResponseDrainTimeout", err)
	}

	if !errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport classification", err)
	}

	if elapsed := time.Since(started); elapsed > 10*successBodyDrainTimeout {
		t.Fatalf("compressed response drain took %s", elapsed)
	}
}

func TestDecodedResponseDrainHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(nil)

	body := newStalledDecodedBody(ctx, `{"requestId":"req-1"}`)
	client := mustNewAPIClient("https://iris.test", "token", WithHTTPClient(&http.Client{}))

	_, err := client.decodeJSONBody[ReplyAcceptedResponse](ctx, cancel, body, http.MethodPost, PathReply, true)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrResponseDrainTimeout) {
		t.Fatalf("decodeJSONBody() error = %v, want context.Canceled without drain timeout", err)
	}
}

func TestDecodedResponseTrailerRejectsExcessAndNonWhitespace(t *testing.T) {
	t.Parallel()

	client := mustNewAPIClient("https://iris.test", "token", WithHTTPClient(&http.Client{}))

	for _, test := range []struct {
		name    string
		trailer string
		want    error
	}{
		{name: "excess", trailer: strings.Repeat(" ", int(decodedBodyDrainMaxLen)+1), want: ErrResponseTooLarge},
		{name: "second value", trailer: ` {"unexpected":true}`, want: errUnexpectedResponseBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)

			body := io.NopCloser(strings.NewReader(`{"requestId":"req-1"}` + test.trailer))
			if _, err := client.decodeJSONBody[ReplyAcceptedResponse](ctx, cancel, body, http.MethodPost, PathReply, true); !errors.Is(err, test.want) {
				t.Fatalf("decodeJSONBody() error = %v, want %v", err, test.want)
			} else if !errors.Is(err, ErrTransport) {
				t.Fatalf("decodeJSONBody() error = %v, want ErrTransport classification", err)
			}
		})
	}
}

func TestPostResponseDecodeFailureBecomesTransportError(t *testing.T) {
	t.Parallel()

	injectedReadErr := errors.New("injected response read failure")
	for _, test := range []struct {
		name string
		body io.ReadCloser
	}{
		{
			name: "partial json",
			body: io.NopCloser(strings.NewReader(`{"requestId":`)),
		},
		{
			name: "body read failure",
			body: &errorAfterPayloadBody{payload: []byte(`{"requestId":`), err: injectedReadErr},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := mustNewAPIClient("https://iris.test", "token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: test.body}, nil
			})))

			_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")
			if !errors.Is(err, ErrTransport) {
				t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport", err)
			}

			transportErr, ok := errors.AsType[*TransportError](err)
			if !ok {
				t.Fatalf("SendMessageAccepted() error = %T, want *TransportError", err)
			}

			if transportErr.Op != "post" || transportErr.URL != PathReply {
				t.Fatalf("TransportError = {Op:%q URL:%q}, want {Op:post URL:%q}", transportErr.Op, transportErr.URL, PathReply)
			}
		})
	}
}

// stalledDecodedBody는 payload 뒤에서 멈추는 본문이다. 실제 transport처럼 요청 context가
// 취소되면 Read를 풀되, HTTP/3처럼 context 오류가 아닌 값을 돌려준다.
type stalledDecodedBody struct {
	ctx     context.Context //nolint:containedctx // 요청 context 취소로 Read가 풀리는 transport 본문을 흉내 낸다.
	payload []byte
	sent    bool
	done    chan struct{}
	closed  atomic.Bool
}

type errorAfterPayloadBody struct {
	payload []byte
	err     error
	sent    bool
	closed  atomic.Bool
}

func (b *errorAfterPayloadBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true

		return copy(p, b.payload), nil
	}

	return 0, b.err
}

func (b *errorAfterPayloadBody) Close() error {
	b.closed.Store(true)

	return nil
}

func newStalledDecodedBody(ctx context.Context, payload string) *stalledDecodedBody {
	return &stalledDecodedBody{ctx: ctx, payload: []byte(payload), done: make(chan struct{})}
}

func (b *stalledDecodedBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true

		return copy(p, b.payload), nil
	}

	select {
	case <-b.done:
	case <-b.ctx.Done():
	}

	return 0, io.ErrClosedPipe
}

func (b *stalledDecodedBody) Close() error {
	if b.closed.CompareAndSwap(false, true) {
		close(b.done)
	}

	return nil
}

type countingReader struct {
	*strings.Reader

	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)

	r.bytesRead += int64(n)

	if err == io.EOF {
		return n, err
	}

	return n, err
}
