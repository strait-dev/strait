package worker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"strait/internal/domain"
)

// classifyError edge cases.

func TestClassifyError_Nil(t *testing.T) {
	t.Parallel()
	if got := classifyError(nil); got != "unknown" {
		t.Fatalf("classifyError(nil) = %q, want %q", got, "unknown")
	}
}

func TestClassifyError_RateLimited(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 429, Body: "rate limited"}
	if got := classifyError(err); got != "rate_limited" {
		t.Fatalf("classifyError(429) = %q, want %q", got, "rate_limited")
	}
}

func TestClassifyError_AuthUnauthorized(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 401, Body: "unauthorized"}
	if got := classifyError(err); got != "auth" {
		t.Fatalf("classifyError(401) = %q, want %q", got, "auth")
	}
}

func TestClassifyError_AuthForbidden(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 403, Body: "forbidden"}
	if got := classifyError(err); got != "auth" {
		t.Fatalf("classifyError(403) = %q, want %q", got, "auth")
	}
}

func TestClassifyError_ClientError(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 400, Body: "bad request"}
	if got := classifyError(err); got != "client" {
		t.Fatalf("classifyError(400) = %q, want %q", got, "client")
	}
}

func TestClassifyError_ClientError422(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 422, Body: "unprocessable"}
	if got := classifyError(err); got != "client" {
		t.Fatalf("classifyError(422) = %q, want %q", got, "client")
	}
}

func TestClassifyError_ServerError(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 500, Body: "internal"}
	if got := classifyError(err); got != "server" {
		t.Fatalf("classifyError(500) = %q, want %q", got, "server")
	}
}

func TestClassifyError_ServerError502(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 502, Body: "bad gateway"}
	if got := classifyError(err); got != "server" {
		t.Fatalf("classifyError(502) = %q, want %q", got, "server")
	}
}

func TestClassifyError_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	if got := classifyError(context.DeadlineExceeded); got != "transient" {
		t.Fatalf("classifyError(DeadlineExceeded) = %q, want %q", got, "transient")
	}
}

func TestClassifyError_ContextCanceled(t *testing.T) {
	t.Parallel()
	if got := classifyError(context.Canceled); got != "transient" {
		t.Fatalf("classifyError(Canceled) = %q, want %q", got, "transient")
	}
}

func TestClassifyError_NetworkError(t *testing.T) {
	t.Parallel()
	err := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	if got := classifyError(err); got != "transient" {
		t.Fatalf("classifyError(net.OpError) = %q, want %q", got, "transient")
	}
}

func TestClassifyError_GenericError(t *testing.T) {
	t.Parallel()
	if got := classifyError(errors.New("something went wrong")); got != "unknown" {
		t.Fatalf("classifyError(generic) = %q, want %q", got, "unknown")
	}
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
		{"server", true},
		{"transient", true},
		{"rate_limited", true},
		{"unknown", true},
	}
	for _, tt := range tests {
		if got := shouldRetryForClass(tt.class); got != tt.want {
			t.Errorf("shouldRetryForClass(%q) = %v, want %v", tt.class, got, tt.want)
		}
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
		{"server", false},
		{"client", false},
		{"auth", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		if got := shouldUseFallbackForClass(tt.class); got != tt.want {
			t.Errorf("shouldUseFallbackForClass(%q) = %v, want %v", tt.class, got, tt.want)
		}
	}
}

// NextRetryDelayWithPolicy edge cases.

func TestNextRetryDelayWithPolicy_ExponentialDefault(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(1, "", 0, 0)
	// empty policy defaults to exponential, zero initial defaults to 1s.
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Fatalf("expected ~1s, got %v", delay)
	}
}

func TestNextRetryDelayWithPolicy_ExponentialGrowth(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(3, domain.RetryBackoffExponential, 2, 3600)
	// attempt 3 with base 2s: 2s -> 4s -> 8s (before jitter).
	if delay < 6400*time.Millisecond || delay > 9600*time.Millisecond {
		t.Fatalf("expected ~8s for attempt 3 with 2s base, got %v", delay)
	}
}

func TestNextRetryDelayWithPolicy_FixedPolicy(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(5, domain.RetryBackoffFixed, 10, 3600)
	// fixed policy: always returns initialDelaySecs (10s) regardless of attempt.
	if delay < 8*time.Second || delay > 12*time.Second {
		t.Fatalf("expected ~10s for fixed policy, got %v", delay)
	}
}

func TestNextRetryDelayWithPolicy_NegativeInputs(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(-1, domain.RetryBackoffExponential, -5, -10)
	// negative attempt floors to 1, negative initial floors to 1s, negative max floors to 3600s.
	if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
		t.Fatalf("expected ~1s with negative inputs, got %v", delay)
	}
}

func TestNextRetryDelayWithPolicy_CapsAtMax(t *testing.T) {
	t.Parallel()
	delay := NextRetryDelayWithPolicy(100, domain.RetryBackoffExponential, 1, 30)
	// 100 attempts with 30s max should be capped at 30s + 20% jitter.
	maxAllowed := 36 * time.Second // 30s + 20% jitter
	if delay > maxAllowed {
		t.Fatalf("expected delay <= %v, got %v", maxAllowed, delay)
	}
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
			if got != tt.expected {
				t.Errorf("durationMillisecondsAtLeastOne(%v) = %d, want %d", tt.d, got, tt.expected)
			}
		})
	}
}
