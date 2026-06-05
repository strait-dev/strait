package worker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// classifyError edge cases.

func TestClassifyError_Nil(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"unknown",
		classifyError(nil))

}

func TestClassifyError_RateLimited(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 429, Body: "rate limited"}
	require.Equal(t,
		"rate_limited",

		classifyError(err))

}

func TestClassifyError_AuthUnauthorized(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 401, Body: "unauthorized"}
	require.Equal(t,
		"auth", classifyError(err))

}

func TestClassifyError_AuthForbidden(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 403, Body: "forbidden"}
	require.Equal(t,
		"auth", classifyError(err))

}

func TestClassifyError_ClientError(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 400, Body: "bad request"}
	require.Equal(t,
		"client",
		classifyError(err))

}

func TestClassifyError_ClientError422(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 422, Body: "unprocessable"}
	require.Equal(t,
		"client",
		classifyError(err))

}

func TestClassifyError_ServerError(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 500, Body: "internal"}
	require.Equal(t,
		"server",
		classifyError(err))

}

func TestClassifyError_ServerError502(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 502, Body: "bad gateway"}
	require.Equal(t,
		"server",
		classifyError(err))

}

func TestClassifyError_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"timeout",
		classifyError(context.
			DeadlineExceeded,
		))

}

func TestClassifyError_ContextCanceled(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"transient",
		classifyError(
			context.
				Canceled,
		))

}

func TestClassifyError_NetworkError(t *testing.T) {
	t.Parallel()
	err := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	require.Equal(t,
		"connection",
		classifyError(err))

}

func TestClassifyError_GenericError(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"unknown",
		classifyError(errors.New(
			"something went wrong")))

}

// shouldRetryForClass and shouldUseFallbackForClass edge cases.

func TestShouldRetryForClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		class string
		want  bool
	}{
		{"client", false},
		{"auth", false},
		{"budget", false},
		{"oom", false},
		{"server", true},
		{"transient", true},
		{"rate_limited", true},
		{"timeout", true},
		{"connection", true},
		{"unknown", true},
	}
	for _, tt := range tests {
		assert.Equal(t,
			tt.want, shouldRetryForClass(tt.class))

	}
}

func TestShouldUseFallbackForClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		class string
		want  bool
	}{
		{"transient", true},
		{"rate_limited", true},
		{"connection", true},
		{"timeout", true},
		{"server", false},
		{"client", false},
		{"auth", false},
		{"budget", false},
		{"oom", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		assert.Equal(t,
			tt.want, shouldUseFallbackForClass(tt.
				class))

	}
}

// NextRetryDelayWithPolicy edge cases.

func TestNextRetryDelayWithPolicy_ExponentialDefault(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(1, "", 0, 0)
	require.False(t,
		delay < 800*
			time.
				Millisecond ||
			delay >
				1200*time.Millisecond)

	// empty policy defaults to exponential, zero initial defaults to 1s.

}

func TestNextRetryDelayWithPolicy_ExponentialGrowth(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(3, domain.RetryBackoffExponential, 2, 3600)
	require.False(t,
		delay < 6400*
			time.
				Millisecond ||
			delay >
				9600*time.Millisecond,
	)

	// attempt 3 with base 2s: 2s -> 4s -> 8s (before jitter).

}

func TestNextRetryDelayWithPolicy_FixedPolicy(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(5, domain.RetryBackoffFixed, 10, 3600)
	require.False(t,
		delay < 8*
			time.Second ||
			delay >
				12*
					time.Second)

	// fixed policy: always returns initialDelaySecs (10s) regardless of attempt.

}

func TestNextRetryDelayWithPolicy_NegativeInputs(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(-1, domain.RetryBackoffExponential, -5, -10)
	require.False(t,
		delay < 800*
			time.
				Millisecond ||
			delay >
				1200*time.Millisecond)

	// negative attempt floors to 1, negative initial floors to 1s, negative max floors to 3600s.

}

func TestNextRetryDelayWithPolicy_CapsAtMax(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(100, domain.RetryBackoffExponential, 1, 30)
	// 100 attempts with 30s max should be capped at 30s + 20% jitter.
	maxAllowed := 36 * time.Second
	require.LessOrEqual(t, delay,
		maxAllowed,
	)

	// 30s + 20% jitter

}

func TestDurationMillisecondsAtLeastOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		d        time.Duration
		expected int64
	}{
		{"zero duration", 0, 0},
		{"sub-millisecond", 500 * time.Microsecond, 1},
		{"one millisecond", time.Millisecond, 1},
		{"100 milliseconds", 100 * time.Millisecond, 100},
		{"1 second", time.Second, 1000},
		{"negative duration", -time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := durationMillisecondsAtLeastOne(tt.d)
			assert.Equal(t,
				tt.expected,
				got)

		})
	}
}
