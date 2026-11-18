package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

type lifetimeReaderAt struct {
	calls   atomic.Int32
	active  atomic.Bool
	started chan struct{}
	release chan struct{}
}

func (r *lifetimeReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if r.calls.Add(1) == 3 {
		r.active.Store(true)
		close(r.started)
		<-r.release
		r.active.Store(false)
	}

	if offset >= 1 {
		return 0, io.EOF
	}

	return copy(p, "x"), nil
}

func TestSendFileJoinsSourceReads(t *testing.T) {
	for _, scenario := range []string{"early response", "cancellation", "retry", "epoch refresh"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) { checkFileSourceLifetime(t, scenario) })
		})
	}
}

func fileLifetimeTransport(t *testing.T, scenario string, source *lifetimeReaderAt, cancel context.CancelFunc, requests *[]*http.Request, firstBody chan<- io.ReadCloser) http.RoundTripper {
	t.Helper()

	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			return epochResponse(http.StatusOK, epochDocument(testNextAuthEpoch)), nil
		}

		*requests = append(*requests, req)
		if len(*requests) == 1 {
			firstBody <- req.Body

			go func() {
				defer req.Body.Close()

				if _, err := io.Copy(io.Discard, req.Body); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("upload: %v", err)
				}
			}()

			<-source.started

			switch scenario {
			case "cancellation":
				cancel()

				return nil, context.Canceled
			case "retry":
				return epochResponse(http.StatusTooManyRequests, `{}`), nil
			case "epoch refresh":
				return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
			}
		}

		return epochResponse(http.StatusOK, `{"success":true,"delivery":"queued","requestId":"file-lifetime","room":"room","type":"file"}`), nil
	})
}

func checkFileSourceLifetime(t *testing.T, scenario string) {
	t.Helper()

	source := &lifetimeReaderAt{started: make(chan struct{}), release: make(chan struct{})}

	ctx, cancel := context.WithCancel(t.Context())

	defer cancel()

	var requests []*http.Request

	firstBody := make(chan io.ReadCloser, 1)

	rt := fileLifetimeTransport(t, scenario, source, cancel, &requests, firstBody)
	client := mustNewAPIClient("https://iris.test", testEpochSecret, withRoundTripper(rt), WithReplyRetry(2))
	returned := make(chan error, 1)

	go func() {
		_, err := client.SendFile(ctx, "room", NewReplyFile("x.txt", "text/plain", 1, source), WithClientRequestID("file-lifetime"))
		returned <- err
	}()

	synctest.Wait()

	select {
	case err := <-returned:
		t.Fatalf("SendFile returned with an active source read: %v", err)
	default:
	}

	if !source.active.Load() {
		t.Fatal("source read is not active")
	}

	if err := (<-firstBody).Close(); err != nil {
		t.Fatal(err)
	}

	close(source.release)
	synctest.Wait()

	assertFileSourceReturn(t, scenario, <-returned, requests, source)
}

func assertFileSourceClosed(t *testing.T, body io.ReadCloser) {
	t.Helper()

	_, readErr := io.Copy(io.Discard, body)
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}

	if !errors.Is(readErr, io.ErrClosedPipe) {
		t.Fatalf("late body read error = %v, want closed source", readErr)
	}
}

func assertFileSourceReturn(t *testing.T, scenario string, err error, requests []*http.Request, source *lifetimeReaderAt) {
	t.Helper()

	if scenario == "cancellation" {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SendFile error = %v, want cancellation", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}

	wantRequests := 1

	if scenario == "retry" || scenario == "epoch refresh" {
		wantRequests = 2
	}

	if len(requests) != wantRequests {
		t.Fatalf("request count = %d, want %d", len(requests), wantRequests)
	}

	calls := source.calls.Load()

	for _, req := range requests {
		assertFileSourceClosed(t, req.Body)

		body, err := req.GetBody()
		if err != nil {
			t.Fatal(err)
		}

		assertFileSourceClosed(t, body)
	}

	if got := source.calls.Load(); got != calls {
		t.Fatalf("source calls after return = %d, want %d", got, calls)
	}
}

func TestImageBodyCannotReadCallerBytesAfterSendReturns(t *testing.T) {
	t.Parallel()

	var request *http.Request

	client := mustNewAPIClient("https://iris.test", testEpochSecret, withRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		request = req

		return epochResponse(http.StatusOK, `{"success":true,"delivery":"queued","requestId":"image-lifetime","room":"room","type":"image"}`), nil
	})))
	payload := []byte{0x89, 'P', 'N', 'G', 0, 1}

	if _, err := client.SendImage(t.Context(), "room", payload); err != nil {
		t.Fatal(err)
	}

	clear(payload)

	assertFileSourceClosed(t, request.Body)

	replay, err := request.GetBody()
	if err != nil {
		t.Fatal(err)
	}

	assertFileSourceClosed(t, replay)
}
