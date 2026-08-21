package webhook

import (
	"math"
	"strconv"
	"testing"
	"time"
)

const replayWindowNowMs = 1787000000000

type replayWindowCase struct {
	name string
	ts   int64
	want bool
}

func runReplayWindowCases(t *testing.T, now time.Time, window time.Duration, cases []replayWindowCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := timestampWithinReplayWindow(strconv.FormatInt(tc.ts, 10), window, now)
			if got != tc.want {
				t.Fatalf("timestampWithinReplayWindow(%d, %s, %s) = %v, want %v",
					tc.ts, window, now.Format(time.RFC3339Nano), got, tc.want)
			}
		})
	}
}

func TestTimestampWithinReplayWindowRejectsOverflowingFutureTimestamps(t *testing.T) {
	t.Parallel()

	now := time.UnixMilli(replayWindowNowMs)
	window := 5 * time.Minute
	windowMs := window.Milliseconds()

	runReplayWindowCases(t, now, window, []replayWindowCase{
		{"now", replayWindowNowMs, true},
		{"lower boundary", replayWindowNowMs - windowMs, true},
		{"upper boundary", replayWindowNowMs + windowMs, true},
		{"one millisecond before lower boundary", replayWindowNowMs - windowMs - 1, false},
		{"one millisecond after upper boundary", replayWindowNowMs + windowMs + 1, false},
		{"year 2027", 1800000000000, false},
		{"year 2286", 9999999999999, false},
		{"year 5138", 99999999999999, false},
		{"largest representable future delta", replayWindowNowMs + 9223372036854, false},
		{"smallest overflowing future delta", replayWindowNowMs + 9223372036855, false},
		{"max int64", math.MaxInt64, false},
		{"largest representable past delta", replayWindowNowMs - 9223372036854, false},
		{"smallest overflowing past delta", replayWindowNowMs - 9223372036855, false},
		{"min int64", math.MinInt64, false},
	})
}

func TestTimestampWithinReplayWindowKeepsSubMillisecondBoundary(t *testing.T) {
	t.Parallel()

	now := time.UnixMilli(replayWindowNowMs).Add(500 * time.Microsecond)
	window := 5 * time.Minute
	windowMs := window.Milliseconds()

	runReplayWindowCases(t, now, window, []replayWindowCase{
		{"lower boundary millisecond falls outside", replayWindowNowMs - windowMs, false},
		{"one millisecond inside lower boundary", replayWindowNowMs - windowMs + 1, true},
		{"upper boundary millisecond falls inside", replayWindowNowMs + windowMs, true},
		{"one millisecond past upper boundary", replayWindowNowMs + windowMs + 1, false},
	})
}

func TestTimestampWithinReplayWindowRejectsUnparsableTimestamps(t *testing.T) {
	t.Parallel()

	now := time.UnixMilli(replayWindowNowMs)
	for _, ts := range []string{"", " ", "abc", "1787000000000.0", "+1787000000000 ", "0x1", "99999999999999999999999999"} {
		if timestampWithinReplayWindow(ts, 5*time.Minute, now) {
			t.Fatalf("timestampWithinReplayWindow(%q) = true, want false", ts)
		}
	}
}
