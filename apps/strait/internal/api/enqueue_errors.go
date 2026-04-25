package api

import (
	"math"
	"net/http"
	"strconv"

	"strait/internal/queue"
)

func enqueueAPIError(err error) error {
	throttled, ok := queue.AsThrottled(err)
	if !ok {
		return nil
	}

	retryAfterSeconds := 1
	if throttled.RetryAfter > 0 {
		retryAfterSeconds = max(int(math.Ceil(throttled.RetryAfter.Seconds())), 1)
	}

	return &typedAPIError{
		status: http.StatusTooManyRequests,
		apiError: APIError{
			Code:    ErrorCodeEnqueueThrottled,
			Message: "enqueue throttled",
			Details: []string{"retry_after_seconds=" + strconv.Itoa(retryAfterSeconds)},
		},
		headers: map[string]string{
			"Retry-After": strconv.Itoa(retryAfterSeconds),
		},
	}
}
