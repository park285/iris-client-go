package client

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxReplyRetryAfterDelay = 5 * time.Second

func parseRetryAfterHeader(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}

	delay := when.Sub(now)
	if delay <= 0 {
		return 0
	}

	return delay
}

func retryDelayForError(err error, fallback time.Duration) time.Duration {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr != nil && httpErr.RetryAfter > 0 {
		return clampDuration(httpErr.RetryAfter, fallback, maxReplyRetryAfterDelay)
	}

	return fallback
}

func clampDuration(value, minValue, maxValue time.Duration) time.Duration {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
