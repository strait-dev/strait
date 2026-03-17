package compute

import (
	"errors"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"retryable 429", NewRetryableError(429, "rate limit", nil), true},
		{"retryable 503", NewRetryableError(503, "capacity", nil), true},
		{"retryable 500", NewRetryableError(500, "server error", nil), true},
		{"fatal 422", NewFatalError(422, "config", nil), false},
		{"regular error", errors.New("oops"), false},
	}
	for _, tt := range tests {
		if got := IsRetryable(tt.err); got != tt.want {
			t.Errorf("%s: IsRetryable() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsFatal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"fatal 422", NewFatalError(422, "config", nil), true},
		{"retryable 429", NewRetryableError(429, "rate limit", nil), false},
		{"regular error", errors.New("oops"), false},
	}
	for _, tt := range tests {
		if got := IsFatal(tt.err); got != tt.want {
			t.Errorf("%s: IsFatal() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBackoffHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  error
		want time.Duration
	}{
		{NewRetryableError(429, "", nil), 10 * time.Second},
		{NewRetryableError(503, "", nil), 30 * time.Second},
		{NewRetryableError(500, "", nil), 5 * time.Second},
		{errors.New("other"), 5 * time.Second},
	}
	for _, tt := range tests {
		if got := BackoffHint(tt.err); got != tt.want {
			t.Errorf("BackoffHint(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestRuntimeError_Error(t *testing.T) {
	t.Parallel()
	err := NewRetryableError(429, "rate limit", errors.New("underlying"))
	msg := err.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRuntimeError_Unwrap(t *testing.T) {
	t.Parallel()
	cause := errors.New("root cause")
	err := NewRetryableError(500, "server error", cause)
	if !errors.Is(err, cause) {
		t.Error("Unwrap should return cause")
	}
}
