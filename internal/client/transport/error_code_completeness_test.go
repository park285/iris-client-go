package transport

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReadErrorBodyRequiresCompleteInBudgetBody(t *testing.T) {
	t.Parallel()

	const limit = 64 << 10

	for _, size := range []int{limit - 1, limit, limit + 1, 2 * limit} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			t.Parallel()

			body := &countingReader{Reader: strings.NewReader(strings.Repeat(" ", size))}
			payload, complete := readErrorBody(body)

			if complete != (size <= limit) {
				t.Fatalf("complete = %t for %d bytes", complete, size)
			}

			if len(payload) != min(size, limit+1) {
				t.Fatalf("payload bytes = %d, want %d", len(payload), min(size, limit+1))
			}

			if want := int64(min(size, limit+1+httpErrorBodyDrainMaxLen)); body.bytesRead != want {
				t.Fatalf("read bytes = %d, want bounded parse plus drain %d", body.bytesRead, want)
			}
		})
	}
}

func TestReadErrorResponseRejectsCodeAfterReadError(t *testing.T) {
	t.Parallel()

	const payload = `{"code":"CLIENT_REQUEST_ID_FAILED"}`

	for _, readErr := range []error{io.ErrUnexpectedEOF, errors.New("injected read failure")} {
		t.Run(readErr.Error(), func(t *testing.T) {
			t.Parallel()

			body := &errorAfterPayloadBody{payload: []byte(payload), err: readErr}
			err := readErrorResponse(PathReply, &http.Response{StatusCode: http.StatusConflict, Body: body})

			if code := HTTPErrorCode(err); code != "" {
				t.Fatalf("code = %q after %v, want empty", code, readErr)
			}

			httpErr, ok := errors.AsType[*HTTPError](err)
			if !ok || httpErr.StatusCode != http.StatusConflict || httpErr.Body != payload {
				t.Fatalf("error = %v, want retained HTTP conflict and diagnostic payload", err)
			}

			if errors.Is(err, ErrRetryable) {
				t.Fatalf("incomplete conflict must not introduce retry policy: %v", err)
			}
		})
	}
}
