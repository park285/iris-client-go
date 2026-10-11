package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRawJSONRequiresOneCompleteValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		body  string
		valid bool
	}{
		{name: "preserve_whitespace_and_numbers", body: " \n{\"x\":1e+09,\"y\": 9007199254740993,\"s\":\"\\u0061\"}\t\r\n", valid: true},
		{name: "array", body: `[true,null,"가🙂"]`, valid: true},
		{name: "null", body: `null`, valid: true},
		{name: "string", body: `"hello"`, valid: true},
		{name: "number", body: `1e+9999`, valid: true},
		{name: "boolean", body: `true`, valid: true},
		{name: "empty"},
		{name: "whitespace", body: " \r\n\t"},
		{name: "incomplete_object", body: `{"x":1`},
		{name: "incomplete_array", body: `[1,`},
		{name: "incomplete_string", body: `"hello`},
		{name: "incomplete_number", body: `1e`},
		{name: "incomplete_literal", body: `tru`},
		{name: "second_object", body: `{} {}`},
		{name: "second_scalar", body: "{}\nnull"},
		{name: "adjacent_values", body: `truefalse`},
		{name: "trailing_garbage", body: `{} nope`},
		{name: "duplicate_name", body: `{"x":1,"x":2}`},
		{name: "escaped_duplicate_name", body: `{"x":1,"\u0078":2}`},
		{name: "invalid_utf8", body: "{\"x\":\"\xff\"}"},
		{name: "invalid_surrogate", body: `"\ud800"`},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				t.Parallel()

				body := &responseLimitBody{Reader: strings.NewReader(tc.body)}
				attempts := 0
				client := NewAPIClient("https://iris.test", "synthetic-test-token", WithReplyRetry(3), WithRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
					attempts++

					if req.Method != method {
						t.Errorf("method = %q, want %q", req.Method, method)
					}

					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				})))

				t.Cleanup(func() { closeRawJSONClient(t, client) })

				raw, err := client.rawJSON(t.Context(), method, PathDiagnosticsRuntime)
				if tc.valid {
					if err != nil || string(raw) != tc.body {
						t.Fatalf("raw = %q, error = %v, want unchanged %q", raw, err, tc.body)
					}
				} else if err == nil || raw != nil || errors.Is(err, ErrRetryable) {
					t.Fatalf("invalid response raw = %q, error = %v", raw, err)
				}

				if attempts != 1 || body.closes.Load() != 1 {
					t.Fatalf("attempts = %d, closes = %d, want one each", attempts, body.closes.Load())
				}
			})
		}
	}
}

func TestRawJSONByteBoundary(t *testing.T) {
	const limit = 1 << 20

	for _, tc := range []struct {
		name    string
		body    string
		trailer string
	}{
		{name: "below", body: `"` + strings.Repeat("x", limit-3) + `"`},
		{name: "exact", body: `"` + strings.Repeat("x", limit-2) + `"`},
		{name: "exact_with_trailer", body: `"` + strings.Repeat("x", limit-3) + `"`, trailer: "\n"},
		{name: "above_in_trailer", body: `"` + strings.Repeat("x", limit-2) + `"`, trailer: "\n"},
		{name: "far_above", body: `"` + strings.Repeat("x", 2*limit) + `"`},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				body := &responseLimitBody{Reader: io.MultiReader(strings.NewReader(tc.body), strings.NewReader(tc.trailer))}
				attempts := 0
				client := NewAPIClient("https://iris.test", "synthetic-test-token", WithReplyRetry(3), WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
					attempts++

					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				})))

				t.Cleanup(func() { closeRawJSONClient(t, client) })

				raw, err := client.rawJSON(t.Context(), method, PathDiagnosticsRuntime)
				size := len(tc.body) + len(tc.trailer)

				if size > limit {
					if raw != nil || !errors.Is(err, ErrResponseTooLarge) || errors.Is(err, ErrRetryable) {
						t.Fatalf("oversize raw present = %t, error = %v", raw != nil, err)
					}
				} else if err != nil || string(raw) != tc.body+tc.trailer {
					t.Fatalf("supported raw response bytes = %d, error = %v", len(raw), err)
				}

				if got := body.bytesRead.Load(); got != int64(min(size, limit+1)) {
					t.Fatalf("read = %d, want %d", got, min(size, limit+1))
				}

				if attempts != 1 || body.closes.Load() != 1 {
					t.Fatalf("attempts = %d, closes = %d, want one each", attempts, body.closes.Load())
				}
			})
		}
	}
}

func closeRawJSONClient(t *testing.T, client *APIClient) {
	t.Helper()

	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
