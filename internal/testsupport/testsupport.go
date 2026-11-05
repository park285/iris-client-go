package testsupport

import (
	jsonv2 "encoding/json/v2"
	"io"
	"net/http"
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

// ServeAuthEpoch는 보호 API fixture와 같은 서버에서 고정된 기동 epoch를 제공한다.
func ServeAuthEpoch(tb testing.TB, w http.ResponseWriter, r *http.Request) bool {
	tb.Helper()

	if r.Method != http.MethodGet || r.URL.Path != "/auth/epoch" {
		return false
	}

	w.Header().Set("Content-Type", "application/json")
	WriteResponse(tb, w, `{"epoch":"000102030405060708090a0b0c0d0e0f"}`)

	return true
}
