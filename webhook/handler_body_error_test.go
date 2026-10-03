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
	payload, ok := handler.decodeAndValidate(response, req)

	assertResponseCode(t, response.Code, http.StatusRequestTimeout)

	if payload != nil || ok {
		t.Fatalf("payload/valid = %v/%v, want nil/false after body deadline", payload, ok)
	}
}
