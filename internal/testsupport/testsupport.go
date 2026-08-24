package testsupport

import (
	jsonv2 "encoding/json/v2"
	"io"
	"testing"
)

func CloseOnCleanup(tb testing.TB, name string, closeFn func() error) {
	tb.Helper()

	tb.Cleanup(func() {
		if err := closeFn(); err != nil {
			tb.Errorf("%s error = %v", name, err)
		}
	})
}

func CloseNow(tb testing.TB, name string, closeFn func() error) {
	tb.Helper()

	if err := closeFn(); err != nil {
		tb.Errorf("%s error = %v", name, err)
	}
}

func WriteResponse(tb testing.TB, w io.Writer, body string) {
	tb.Helper()

	if _, err := io.WriteString(w, body); err != nil {
		tb.Errorf("write response error = %v", err)
	}
}

func WriteJSON(tb testing.TB, w io.Writer, value any) {
	tb.Helper()

	if err := jsonv2.MarshalWrite(w, value); err != nil {
		tb.Errorf("write JSON response error = %v", err)
	}
}

func AssertType[T any](tb testing.TB, name string, value any) T {
	tb.Helper()

	typed, ok := value.(T)
	if !ok {
		tb.Fatalf("%s type = %T, want %T", name, value, typed)
	}

	return typed
}
