package client

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRetryBackoffJitterIsNotConstant(t *testing.T) {
	t.Parallel()

	base := 200 * time.Millisecond
	plain := errors.New("transport blip")

	seen := make(map[time.Duration]struct{})
	for range 100 {
		d := retryDelayForError(plain, base)
		seen[d] = struct{}{}
	}

	if len(seen) < 2 {
		t.Fatalf("backoff jitter produced %d distinct delays over 100 samples, want >= 2", len(seen))
	}
}

func TestRetryBackoffJitterBounds(t *testing.T) {
	t.Parallel()

	base := 200 * time.Millisecond
	plain := errors.New("transport blip")

	for range 1000 {
		d := retryDelayForError(plain, base)
		if d < base/2 || d > base {
			t.Fatalf("jittered delay %s out of half-jitter bounds [%s, %s]", d, base/2, base)
		}
	}
}

func TestHalfJitterUsesInjectedSource(t *testing.T) {
	base := 200 * time.Millisecond

	orig := halfJitterFloat64
	t.Cleanup(func() { halfJitterFloat64 = orig })

	halfJitterFloat64 = func() float64 { return 0 }
	if got := halfJitter(base); got != base/2 {
		t.Fatalf("halfJitter with source=0 = %s, want floor %s", got, base/2)
	}

	halfJitterFloat64 = func() float64 { return 0.999999 }
	if got := halfJitter(base); got <= base/2 || got > base {
		t.Fatalf("halfJitter with source~1 = %s, want close to ceil %s", got, base)
	}
}

func TestRetryAfterTakesPriorityOverJitter(t *testing.T) {
	t.Parallel()

	base := 50 * time.Millisecond

	// Retry-After within bounds must be honored verbatim, never jittered.
	withRetryAfter := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: 2 * time.Second})
	for range 100 {
		if got := retryDelayForError(withRetryAfter, base); got != 2*time.Second {
			t.Fatalf("Retry-After delay = %s, want exact 2s (no jitter applied)", got)
		}
	}

	// Short Retry-After clamps to base (the floor), still not jittered.
	short := fmt.Errorf("wrapped: %w", &HTTPError{StatusCode: 429, RetryAfter: time.Millisecond})
	for range 100 {
		if got := retryDelayForError(short, base); got != base {
			t.Fatalf("short Retry-After delay = %s, want clamp to base %s", got, base)
		}
	}
}
