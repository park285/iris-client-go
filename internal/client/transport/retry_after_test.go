package transport

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfterHeaderSeconds(t *testing.T) {
	t.Parallel()

	if got := parseRetryAfterHeader("3", time.Unix(0, 0)); got != 3*time.Second {
		t.Fatalf("parseRetryAfterHeader(seconds) = %s, want 3s", got)
	}
}

func TestParseRetryAfterHeaderHTTPDate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	value := now.Add(2 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfterHeader(value, now); got != 2*time.Second {
		t.Fatalf("parseRetryAfterHeader(date) = %s, want 2s", got)
	}
}

func TestParseRetryAfterHeaderIgnoresInvalidAndPastValues(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	for _, value := range []string{"", "-1", "0", "garbage", now.Add(-time.Second).Format(time.RFC1123)} {
		if got := parseRetryAfterHeader(value, now); got != 0 {
			t.Fatalf("parseRetryAfterHeader(%q) = %s, want 0", value, got)
		}
	}
}

func TestRetryDelayForErrorUsesHTTPRetryAfterWithBounds(t *testing.T) {
	t.Parallel()

	base := 50 * time.Millisecond

	short := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: time.Millisecond})
	if got := retryDelayForError(short, base); got != base {
		t.Fatalf("short Retry-After delay = %s, want base %s", got, base)
	}

	long := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: 30 * time.Second})
	if got := retryDelayForError(long, base); got != maxReplyRetryAfterDelay {
		t.Fatalf("long Retry-After delay = %s, want cap %s", got, maxReplyRetryAfterDelay)
	}

	if got := retryDelayForError(errors.New("plain"), base); got < base/2 || got > base {
		t.Fatalf("plain error delay = %s, want jittered within [%s, %s]", got, base/2, base)
	}
}

func TestRetryDelayForErrorCapsRetryAfterAtMax(t *testing.T) {
	t.Parallel()

	base := 50 * time.Millisecond

	const wantCap = 5 * time.Second
	overCap := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: 10 * time.Second})
	if got := retryDelayForError(overCap, base); got != wantCap {
		t.Fatalf("Retry-After=10s delay = %s, want cap %s", got, wantCap)
	}
}

func TestRetryDelayForErrorHonorsRetryAfterFloorWithinBounds(t *testing.T) {
	t.Parallel()

	base := 200 * time.Millisecond

	for _, retryAfter := range []time.Duration{
		base,
		base + 100*time.Millisecond,
		time.Second,
		5 * time.Second,
	} {
		err := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: retryAfter})
		got := retryDelayForError(err, base)
		if got != retryAfter {
			t.Fatalf("Retry-After=%s delay = %s, want exact %s (in-bounds value honored verbatim)", retryAfter, got, retryAfter)
		}
		if got < retryAfter {
			t.Fatalf("Retry-After=%s delay = %s violates server-requested floor", retryAfter, got)
		}
	}
}

func TestReadErrorResponsePreservesRetryAfter(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("rate limited")),
	}
	resp.Header.Set("Retry-After", "3")

	var got *HTTPError
	if !errors.As(readErrorResponse(PathReply, resp), &got) {
		t.Fatal("readErrorResponse() did not return *HTTPError")
	}
	if got.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %s, want 3s", got.RetryAfter)
	}
}
