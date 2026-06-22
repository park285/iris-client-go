package client

import (
	"errors"
	"strings"
	"testing"
)

func TestErrRetryable_MatchesWrappedHTTPError(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 503, URL: "http://iris.test/reply"}
	var err error = httpErr

	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected errors.Is(err, ErrRetryable) to be true, got false")
	}

	var got *HTTPError
	if !errors.As(err, &got) {
		t.Fatalf("expected errors.As to extract *HTTPError, failed")
	}
	if got.StatusCode != 503 {
		t.Fatalf("StatusCode=%d, want 503", got.StatusCode)
	}
}

func TestErrPermanent_DoesNotMatchRetryable(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 400, URL: "http://iris.test/reply"}
	var err error = httpErr

	if errors.Is(err, ErrRetryable) {
		t.Fatalf("400 must not be retryable")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("400 must match ErrPermanent")
	}
}

func TestErrAuthFailed_Matches401(t *testing.T) {
	var err error = &HTTPError{StatusCode: 401, URL: "http://iris.test/reply"}
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("401 must match ErrAuthFailed")
	}
}

func TestErrRateLimited_Matches429(t *testing.T) {
	var err error = &HTTPError{StatusCode: 429, URL: "http://iris.test/reply"}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429 must match ErrRateLimited")
	}
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("429 must also match ErrRetryable (retry with backoff)")
	}
}

func TestTransportError_Init_NotRetryable(t *testing.T) {
	te := &TransportError{Op: opInit, URL: "h3://x", Err: errors.New("CA parse failed")}
	if errors.Is(te, ErrRetryable) {
		t.Fatalf("init-op TransportError must NOT match ErrRetryable")
	}
	if !errors.Is(te, ErrTransport) {
		t.Fatalf("must still match ErrTransport")
	}
}

func TestTransportError_Dial_StillRetryable(t *testing.T) {
	te := &TransportError{Op: "dial", URL: "h3://x", Err: errors.New("connection refused")}
	if !errors.Is(te, ErrRetryable) {
		t.Fatalf("dial-op must match ErrRetryable")
	}
}

func TestTransportError_H3EgressDenied_NotRetryable(t *testing.T) {
	te := &TransportError{Op: "post", URL: "https://iris.test/reply", Err: ErrH3EgressDenied}
	if errors.Is(te, ErrRetryable) {
		t.Fatalf("H3 egress deny must not match ErrRetryable")
	}
	if !errors.Is(te, ErrTransport) {
		t.Fatalf("must still match ErrTransport")
	}
	if !errors.Is(te, ErrH3EgressDenied) {
		t.Fatalf("must expose ErrH3EgressDenied")
	}
}

func TestTransportError_ErrorRedactsURLSecrets(t *testing.T) {
	te := &TransportError{
		Op:  "post",
		URL: "https://user:secret@iris.test/reply?token=abc123&room=42#frag",
		Err: errors.New("connection refused"),
	}

	got := te.Error()
	for _, forbidden := range []string{"user", "secret", "token=", "abc123", "room=42", "#frag"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("TransportError leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "https://iris.test/reply") {
		t.Fatalf("TransportError = %q, want redacted target path", got)
	}
}

func TestTruncateBody_RedactsBearerToken(t *testing.T) {
	in := strings.NewReader("error context Bearer abcdef1234567890 trailing")
	got := truncateBody(in)
	if strings.Contains(got, "abcdef1234567890") {
		t.Fatalf("token leaked: %q", got)
	}
}

func TestRedactSensitiveTokens_RedactsXIrisSecretValue(t *testing.T) {
	in := "error context X-Iris-Secret: abcdef1234567890 trailing"
	got := redactSensitiveTokens(in)
	if strings.Contains(got, "abcdef1234567890") {
		t.Fatalf("X-Iris-Secret value leaked: %q", got)
	}
}

func TestRedactSensitiveTokens_RedactsXIrisSecretWhitespaceVariants(t *testing.T) {
	for _, in := range []string{
		"error context X-Iris-Secret:\tabc123 trailing",
		"error context X-Iris-Secret:abc123 trailing",
	} {
		got := redactSensitiveTokens(in)
		if strings.Contains(got, "abc123") {
			t.Fatalf("X-Iris-Secret value leaked for %q: %q", in, got)
		}
	}
}

func TestRedactSensitiveTokens_CaseInsensitive(t *testing.T) {
	cases := []struct{ in, mustNotContain string }{
		{"authorization: bearer abc123", "abc123"},
		{"Authorization: Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"X-Iris-Token: token-value-xyz", "token-value-xyz"},
		{"X-API-Key: key-12345", "key-12345"},
		{"Cookie: session=secret-cookie", "secret-cookie"},
		{"Set-Cookie: id=abc; HttpOnly", "abc"},
	}
	for _, tc := range cases {
		got := redactSensitiveTokens(tc.in)
		if strings.Contains(got, tc.mustNotContain) {
			t.Fatalf("leaked %q in %q: %q", tc.mustNotContain, tc.in, got)
		}
	}
}

func TestTruncateBody_Caps512Bytes(t *testing.T) {
	in := strings.NewReader(strings.Repeat("x", 2000))
	got := truncateBody(in)
	if len(got) > 512 {
		t.Fatalf("body length %d > 512", len(got))
	}
}
