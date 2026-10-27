package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

type failingWebhookBody struct {
	err error
}

func (b failingWebhookBody) Read([]byte) (int, error) {
	return 0, b.err
}

func (failingWebhookBody) Close() error { return nil }

func TestWebhookBodyReadFailureRejectsBeforeAdmission(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"middleware deadline", context.DeadlineExceeded, http.StatusRequestTimeout},
		{"wrapped middleware deadline", fmt.Errorf("read body: %w", context.DeadlineExceeded), http.StatusRequestTimeout},
		{"socket deadline", os.ErrDeadlineExceeded, http.StatusRequestTimeout},
		{"wrapped socket timeout", fmt.Errorf("read body: %w", &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}), http.StatusRequestTimeout},
		{"body too large", &http.MaxBytesError{Limit: 8}, http.StatusRequestEntityTooLarge},
		{"size limit takes precedence", errors.Join(&http.MaxBytesError{Limit: 8}, context.DeadlineExceeded), http.StatusRequestEntityTooLarge},
		{"incomplete body", io.ErrUnexpectedEOF, http.StatusBadRequest},
		{"canceled body", context.Canceled, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			admitter := &recordingAdmitter{}
			nonceStore := newMemoryNonceCache()
			handler := mustNewDurableHandler(t, admitter, slog.New(slog.DiscardHandler), WithNonceStore(nonceStore))

			defer closeHandler(t, handler)

			req := acceptedCaseRequest(t)

			req.Body = failingWebhookBody{err: tc.err}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)

			assertResponseCode(t, response.Code, tc.status)

			if admitter.calls != 0 || len(nonceStore.entries) != 0 {
				t.Fatalf("admissions/nonces = %d/%d, want 0/0 before complete body verification", admitter.calls, len(nonceStore.entries))
			}
		})
	}
}

func TestWebhookDecodeReadTimeoutDoesNotValidatePartialInput(t *testing.T) {
	t.Parallel()

	handler := mustNewDurableHandler(t, &recordingAdmitter{}, slog.New(slog.DiscardHandler), WithNonceStore(newMemoryNonceCache()))
	defer closeHandler(t, handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", failingWebhookBody{err: context.DeadlineExceeded})
	response := httptest.NewRecorder()
	body, ok := handler.bufferBodyForHMAC(response, req)

	assertResponseCode(t, response.Code, http.StatusRequestTimeout)

	if body != nil || ok {
		t.Fatalf("body/ok = %v/%v, want nil/false after body deadline", body, ok)
	}
}

// 인증 전 버퍼링은 송신자가 선언한 Content-Length를 믿으면 안 된다. 1 byte만 보내고 큰 길이를 선언한
// 미인증 연결이 연결마다 maxBodyBytes만큼 힙을 붙잡게 되기 때문이다. 병렬 테스트가 할당량 측정에
// 섞이지 않도록 t.Parallel을 쓰지 않는다.
func TestBufferBodyForHMACAllocatesForReceivedBytesNotDeclaredLength(t *testing.T) {
	const (
		declaredBodyBytes = 1 << 20
		requests          = 8
	)

	handler := mustNewDurableHandler(t, &recordingAdmitter{}, slog.New(slog.DiscardHandler),
		WithNonceStore(newMemoryNonceCache()), WithMaxBodyBytes(declaredBodyBytes))
	defer closeHandler(t, handler)

	reqs := make([]*http.Request, requests)
	responses := make([]*httptest.ResponseRecorder, requests)

	for i := range requests {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhook/iris", strings.NewReader("{"))

		req.ContentLength = declaredBodyBytes
		reqs[i] = req
		responses[i] = httptest.NewRecorder()
	}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	for i := range requests {
		if _, ok := handler.bufferBodyForHMAC(responses[i], reqs[i]); !ok {
			t.Fatalf("request %d: bufferBodyForHMAC rejected a complete 1 byte body (status %d)", i, responses[i].Code)
		}
	}

	runtime.ReadMemStats(&after)

	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= declaredBodyBytes {
		t.Fatalf("allocated %d bytes for %d one-byte bodies declaring %d bytes, want < %d", allocated, requests, declaredBodyBytes, declaredBodyBytes)
	}
}
