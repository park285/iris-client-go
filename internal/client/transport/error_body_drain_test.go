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

func TestTruncateBodyDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 256<<10))}

	_ = truncateBody(reader)

	maxRead := int64(httpErrorBodyParseMaxLen + httpErrorBodyDrainMaxLen)
	if reader.bytesRead > maxRead {
		t.Fatalf("truncateBody read %d bytes, want at most %d", reader.bytesRead, maxRead)
	}
}

func TestDoRequestSuccessDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 4<<20))}
	c := NewAPIClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)}, nil
	})))

	if err := c.SendMessage(t.Context(), testRoom, "hello"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if reader.bytesRead > successBodyDrainMaxLen {
		t.Fatalf("success drain read %d bytes, want at most %d", reader.bytesRead, int64(successBodyDrainMaxLen))
	}

	if reader.bytesRead == 0 {
		t.Fatal("success drain read 0 bytes; keep-alive reuse needs a best-effort drain")
	}
}

func TestDoRequestDrainsTrailerAfterDecode(t *testing.T) {
	t.Parallel()

	body := `{"requestId":"req-1"}` + strings.Repeat("\n", 64)
	reader := &countingReader{Reader: strings.NewReader(body)}
	c := NewAPIClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
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

	body := newStalledDecodedBody(`{"requestId":"req-1"}`)
	client := NewAPIClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
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

	if elapsed := time.Since(started); elapsed > 10*decodedBodyDrainTimeout {
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

	client := NewAPIClient(server.URL, "token", WithHTTPClient(server.Client()))
	started := time.Now()

	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")
	if !errors.Is(err, ErrResponseDrainTimeout) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrResponseDrainTimeout", err)
	}

	if !errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport classification", err)
	}

	if elapsed := time.Since(started); elapsed > 10*decodedBodyDrainTimeout {
		t.Fatalf("compressed response drain took %s", elapsed)
	}
}

func TestDecodedResponseDrainHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	body := newStalledDecodedBody(`{"requestId":"req-1"}`)
	client := NewAPIClient("https://iris.test", "token")

	if _, err := client.decodeJSONBody[ReplyAcceptedResponse](ctx, body, http.MethodPost, PathReply); !errors.Is(err, context.Canceled) {
		t.Fatalf("decodeJSONBody() error = %v, want context.Canceled", err)
	}
}

func TestDecodedResponseTrailerRejectsExcessAndNonWhitespace(t *testing.T) {
	t.Parallel()

	client := NewAPIClient("https://iris.test", "token")

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

			body := io.NopCloser(strings.NewReader(`{"requestId":"req-1"}` + test.trailer))
			if _, err := client.decodeJSONBody[ReplyAcceptedResponse](t.Context(), body, http.MethodPost, PathReply); !errors.Is(err, test.want) {
				t.Fatalf("decodeJSONBody() error = %v, want %v", err, test.want)
			} else if !errors.Is(err, ErrTransport) {
				t.Fatalf("decodeJSONBody() error = %v, want ErrTransport classification", err)
			}
		})
	}
}

func TestDecodedResponseDrainPanicBecomesTransportError(t *testing.T) {
	t.Parallel()

	body := &panicAfterPayloadBody{payload: []byte(`{"requestId":"req-1"}`)}
	client := NewAPIClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})))

	if _, err := client.SendMessageAccepted(t.Context(), testRoom, "hello"); !errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport", err)
	}

	if !body.closed.Load() {
		t.Fatal("panicking decoded response body was not closed")
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

			client := NewAPIClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
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

type stalledDecodedBody struct {
	payload []byte
	sent    bool
	done    chan struct{}
	closed  atomic.Bool
}

type panicAfterPayloadBody struct {
	payload []byte
	sent    bool
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

func (b *panicAfterPayloadBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true

		return copy(p, b.payload), nil
	}

	panic("injected response body read panic")
}

func (b *panicAfterPayloadBody) Close() error {
	b.closed.Store(true)

	return nil
}

func newStalledDecodedBody(payload string) *stalledDecodedBody {
	return &stalledDecodedBody{payload: []byte(payload), done: make(chan struct{})}
}

func (b *stalledDecodedBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true

		return copy(p, b.payload), nil
	}

	<-b.done

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
		return n, err //nolint:wrapcheck // io·RoundTripper 어댑터는 하위 오류를 그대로 전달하는 계약이다.
	}

	return n, err //nolint:wrapcheck // io·RoundTripper 어댑터는 하위 오류를 그대로 전달하는 계약이다.
}
