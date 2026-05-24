package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors for common domain conditions.
var (
	ErrJobDisabled = errors.New("job is disabled")

	// ErrIdempotencyConflict is returned when an insert fails due to the
	// idempotency unique index constraint (idx_runs_idempotency). Callers
	// should retry by looking up the existing run via GetRunByIdempotencyKey.
	ErrIdempotencyConflict = errors.New("idempotency key conflict")

	// ErrCanaryNotFound is returned when no active canary deployment exists.
	ErrCanaryNotFound = errors.New("no active canary deployment found")
)

// TransitionError is returned when an FSM state transition is invalid.
type TransitionError struct {
	From RunStatus
	To   RunStatus
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid transition: %s -> %s", e.From, e.To)
}

// UnknownStatusError is returned for an unrecognized run status.
type UnknownStatusError struct {
	Status RunStatus
}

func (e *UnknownStatusError) Error() string {
	return fmt.Sprintf("unknown status: %s", e.Status)
}

// EndpointError is returned when a job endpoint responds with a non-2xx status.
type EndpointError struct {
	StatusCode int
	Body       string
}

func (e *EndpointError) Error() string {
	return fmt.Sprintf("endpoint returned %d", e.StatusCode)
}

// FieldError is returned when an unsupported field is used in an update operation.
type FieldError struct {
	Field string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("unsupported update field: %s", e.Field)
}

// ConfigError is returned when configuration validation fails.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config %s: %s", e.Field, e.Message)
}
