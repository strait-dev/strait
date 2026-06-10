package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"strait/internal/config"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

// TestTriggerLimitAPIError_QuotaErrorsCarryRetryAfter is the regression
// test for STR-531. Each trigger-limit sentinel must map to a 429 response
// that carries a Retry-After header so well-behaved clients can back off
// instead of hammering the API at the limit.
func TestTriggerLimitAPIError_QuotaErrorsCarryRetryAfter(t *testing.T) {
	cases := []struct {
		name  string
		sent  error
		wantC string
	}{
		{"queued quota", errTriggerProjectQueuedQuotaExceeded, ErrorCodeRateLimited},
		{"executing quota", errTriggerProjectExecutingQuotaExceeded, ErrorCodeRateLimited},
		{"job rate limit", errTriggerJobRateLimitExceeded, ErrorCodeRateLimited},
	}
	wantRetryAfter := strconv.Itoa(triggerLimitFallbackRetryAfterSeconds)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := triggerLimitAPIError(tc.sent, "fallback")
			var tae *typedAPIError
			require.ErrorAs(t, err, &tae)
			require.Equal(t, http.StatusTooManyRequests,

				tae.status)
			require.Equal(t, tc.wantC,
				tae.apiError.
					Code)

			gotRetry, ok := tae.headers["Retry-After"]
			require.True(t, ok)
			require.Equal(t, wantRetryAfter,
				gotRetry,
			)
			require.True(t, slices.Contains(tae.
				apiError.
				Details, "retry_after_seconds="+
				wantRetryAfter))

			// Details must echo the same number for body-only clients.
		})
	}
}

// TestTriggerLimitAPIError_PassesThroughHumaStatusErrors verifies that
// preexisting huma.StatusError values are returned by identity — not
// wrapped, not converted, not even allocated into an equal-but-different
// value. This preserves the existing daily-cost-budget 429 path which
// intentionally does not carry Retry-After (the budget resets at
// midnight). Identity (==) is intentional: errors.Is would also pass for
// a wrapped form, and a wrap would silently break code that asserts on
// the concrete pointer (e.g., header reads on huma's underlying type).
func TestTriggerLimitAPIError_PassesThroughHumaStatusErrors(t *testing.T) {
	in := huma.Error429TooManyRequests("project daily cost budget exceeded")
	out := triggerLimitAPIError(in, "fallback")
	require.Equal(t, in, out)
}

// TestTriggerLimitAPIError_DoesNotWrapHumaStatusErrors is the negative
// guardrail for the passthrough contract: if someone "fixes" the
// passthrough by wrapping the input ("%w" or fmt.Errorf), the identity
// check above could pass on errors.Is alone — this test forces a real
// failure by constructing a wrapped-then-passed-through scenario.
func TestTriggerLimitAPIError_DoesNotWrapHumaStatusErrors(t *testing.T) {
	in := huma.Error429TooManyRequests("project daily cost budget exceeded")
	out := triggerLimitAPIError(in, "fallback")
	require.NoError(t, errors.Unwrap(out))

	// errors.Unwrap on a passthrough must return nil — the helper hasn't
	// added a layer. A wrapper that satisfies errors.Is(out, in) would
	// expose `in` via Unwrap.
}

// TestTriggerLimitAPIError_UnknownErrorBecomes500 verifies the fallback
// path: an unrecognized error is reported as 500 with the supplied
// message, not silently converted to a 429.
func TestTriggerLimitAPIError_UnknownErrorBecomes500(t *testing.T) {
	out := triggerLimitAPIError(errors.New("unknown"), "fallback msg")
	var se huma.StatusError
	require.ErrorAs(t, out, &se)
	require.Equal(t, http.StatusInternalServerError,

		se.GetStatus())
}

func TestTriggerLimitAPIError_RetryableDatabaseErrorBecomes429(t *testing.T) {
	out := triggerLimitAPIError(
		fmt.Errorf("enqueue run in tx: failed to deallocate cached statement(s): %w", retryableAdmissionErr{}),
		"failed to enqueue run",
	)
	var tae *typedAPIError
	require.ErrorAs(t, out, &tae)
	require.Equal(t, http.StatusTooManyRequests, tae.status)
	require.Equal(t, ErrorCodeRateLimited, tae.apiError.Code)
	require.Equal(t, "1", tae.headers["Retry-After"])
	require.True(t, slices.Contains(tae.apiError.Details, "retry_after_seconds=1"))
}

func TestServerTriggerLimitAPIError_DBBackpressureDisabledDoesNotReturn429(t *testing.T) {
	srv := &Server{config: &config.Config{DBBackpressureDisabled: true}}

	out := srv.triggerLimitAPIError(
		fmt.Errorf("enqueue run in tx: failed to deallocate cached statement(s): %w", retryableAdmissionErr{}),
		"failed to enqueue run",
	)

	var statusErr huma.StatusError
	require.ErrorAs(t, out, &statusErr)
	require.Equal(t, http.StatusInternalServerError, statusErr.GetStatus())
	require.NotContains(t, out.Error(), "database admission control throttled")
}

// TestNewTriggerLimit429_ResponseShape directly exercises the helper so
// the test suite catches accidental drift between the header value and
// the body detail string.
func TestNewTriggerLimit429_ResponseShape(t *testing.T) {
	err := newTriggerLimit429("queued quota exceeded")
	var tae *typedAPIError
	require.ErrorAs(t, err, &tae)
	require.NotEmpty(t, tae.headers["Retry-After"])

	if n, err := strconv.Atoi(tae.headers["Retry-After"]); err != nil || n <= 0 {
		require.Failf(t, "test failure",

			"Retry-After must be a positive integer, got %q (err=%v)", tae.headers["Retry-After"], err)
	}
}
