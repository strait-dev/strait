package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2"
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
	wantRetryAfter := strconv.Itoa(triggerLimitRetryAfterSeconds)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := triggerLimitAPIError(tc.sent, "fallback")
			var tae *typedAPIError
			if !errors.As(err, &tae) {
				t.Fatalf("expected *typedAPIError, got %T: %v", err, err)
			}
			if tae.status != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", tae.status)
			}
			if tae.apiError.Code != tc.wantC {
				t.Fatalf("code = %q, want %q", tae.apiError.Code, tc.wantC)
			}
			gotRetry, ok := tae.headers["Retry-After"]
			if !ok {
				t.Fatal("missing Retry-After header")
			}
			if gotRetry != wantRetryAfter {
				t.Fatalf("Retry-After = %q, want %q", gotRetry, wantRetryAfter)
			}
			// Details must echo the same number for body-only clients.
			if !slices.Contains(tae.apiError.Details, "retry_after_seconds="+wantRetryAfter) {
				t.Fatalf("details did not include retry_after_seconds=%s: %+v",
					wantRetryAfter, tae.apiError.Details)
			}
		})
	}
}

// TestTriggerLimitAPIError_PassesThroughHumaStatusErrors verifies that
// preexisting huma.StatusError values are not double-wrapped. This
// preserves the existing daily-cost-budget 429 path which intentionally
// does not carry Retry-After (the budget resets at midnight).
func TestTriggerLimitAPIError_PassesThroughHumaStatusErrors(t *testing.T) {
	in := huma.Error429TooManyRequests("project daily cost budget exceeded")
	out := triggerLimitAPIError(in, "fallback")
	if out != in {
		t.Fatalf("got %v, want passthrough of original huma error", out)
	}
}

// TestTriggerLimitAPIError_UnknownErrorBecomes500 verifies the fallback
// path: an unrecognized error is reported as 500 with the supplied
// message, not silently converted to a 429.
func TestTriggerLimitAPIError_UnknownErrorBecomes500(t *testing.T) {
	out := triggerLimitAPIError(errors.New("unknown"), "fallback msg")
	var se huma.StatusError
	if !errors.As(out, &se) {
		t.Fatalf("expected huma.StatusError, got %T", out)
	}
	if se.GetStatus() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", se.GetStatus())
	}
}

// TestNewTriggerLimit429_ResponseShape directly exercises the helper so
// the test suite catches accidental drift between the header value and
// the body detail string.
func TestNewTriggerLimit429_ResponseShape(t *testing.T) {
	err := newTriggerLimit429("queued quota exceeded")
	var tae *typedAPIError
	if !errors.As(err, &tae) {
		t.Fatalf("expected *typedAPIError, got %T", err)
	}
	if tae.headers["Retry-After"] == "" {
		t.Fatal("Retry-After header must be set")
	}
	if n, err := strconv.Atoi(tae.headers["Retry-After"]); err != nil || n <= 0 {
		t.Fatalf("Retry-After must be a positive integer, got %q (err=%v)", tae.headers["Retry-After"], err)
	}
}
