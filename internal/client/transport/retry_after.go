package transport

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxReplyRetryAfterDelay = 5 * time.Second

// halfJitterFloat64은 [0,1) 난수원이며 테스트 주입 지점입니다.
var halfJitterFloat64 = rand.Float64

func halfJitter(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	half := base / 2
	return half + time.Duration(halfJitterFloat64()*float64(base-half))
}

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
	delay, _ := retryDelayAndRetryAfter(err, fallback)
	return delay
}

func retryDelayAndRetryAfter(err error, fallback time.Duration) (time.Duration, bool) {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr != nil && httpErr.RetryAfter > 0 {
		return clampDuration(httpErr.RetryAfter, fallback, maxReplyRetryAfterDelay), true
	}

	return halfJitter(fallback), false
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
