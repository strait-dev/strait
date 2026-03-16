package compute

import (
	"errors"
	"fmt"
	"time"
)

// RuntimeError wraps errors from container runtimes with classification.
type RuntimeError struct {
	StatusCode int    // HTTP status code from provider API (0 if not HTTP).
	Message    string // Human-readable error message.
	Retryable  bool   // Whether the operation should be retried.
	Fatal      bool   // Whether the error is permanent (config issue).
	Cause      error  // Underlying error.
}

func (e *RuntimeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("compute: %s (status=%d retryable=%v): %v", e.Message, e.StatusCode, e.Retryable, e.Cause)
	}
	return fmt.Sprintf("compute: %s (status=%d retryable=%v)", e.Message, e.StatusCode, e.Retryable)
}

func (e *RuntimeError) Unwrap() error { return e.Cause }

// IsRetryable returns true if the error should trigger a retry.
func IsRetryable(err error) bool {
	var re *RuntimeError
	if errors.As(err, &re) {
		return re.Retryable
	}
	return false
}

// IsFatal returns true if the error is permanent and retrying won't help.
func IsFatal(err error) bool {
	var re *RuntimeError
	if errors.As(err, &re) {
		return re.Fatal
	}
	return false
}

// BackoffHint returns a suggested delay before retrying.
func BackoffHint(err error) time.Duration {
	var re *RuntimeError
	if !errors.As(err, &re) {
		return 5 * time.Second
	}
	switch re.StatusCode {
	case 429:
		return 10 * time.Second
	case 503:
		return 30 * time.Second
	case 500:
		return 5 * time.Second
	default:
		return 5 * time.Second
	}
}

// NewRetryableError creates a retryable runtime error.
func NewRetryableError(statusCode int, msg string, cause error) *RuntimeError {
	return &RuntimeError{StatusCode: statusCode, Message: msg, Retryable: true, Cause: cause}
}

// NewFatalError creates a non-retryable runtime error.
func NewFatalError(statusCode int, msg string, cause error) *RuntimeError {
	return &RuntimeError{StatusCode: statusCode, Message: msg, Fatal: true, Cause: cause}
}
