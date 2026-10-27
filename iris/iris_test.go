package iris

import (
	"errors"
	"reflect"
	"testing"
)

func TestFacadeReexportsErrorContracts(t *testing.T) {
	t.Parallel()

	if fields := reflect.TypeFor[HTTPError]().NumField(); fields != 4 {
		t.Fatalf("HTTPError field count = %d, want stable v1 layout with 4 fields", fields)
	}

	err := &HTTPError{StatusCode: 503, URL: "/reply", Body: "request failed"}
	if !errors.Is(err, ErrRetryable) {
		t.Fatal("HTTPError 503 must match ErrRetryable through facade")
	}

	if _, ok := errors.AsType[*HTTPError](err); !ok {
		t.Fatal("HTTPError alias must be extractable through facade")
	}

	if code := HTTPErrorCode(err); code != "" {
		t.Fatalf("HTTPErrorCode() = %q for a plain HTTPError, want empty", code)
	}

	if HTTPErrorCodeClientRequestIDFailed != "CLIENT_REQUEST_ID_FAILED" {
		t.Fatal("public Iris conflict code changed")
	}
}
