package strait

import "fmt"

// TransportError represents a network or transport-level failure.
type TransportError struct {
	Message string
	Cause   error
}

func (e *TransportError) Error() string { return e.Message }
func (e *TransportError) Unwrap() error { return e.Cause }

// DecodeError represents a JSON decode failure.
type DecodeError struct {
	Message string
	Body    string
	Cause   error
}

func (e *DecodeError) Error() string { return e.Message }
func (e *DecodeError) Unwrap() error { return e.Cause }

// ValidationError represents a config or input validation failure.
type ValidationError struct {
	Message string
	Issues  []string
}

func (e *ValidationError) Error() string { return e.Message }

// UnauthorizedError represents a 401 or 403 HTTP error.
type UnauthorizedError struct {
	Status  int
	Message string
	Body    any
}

func (e *UnauthorizedError) Error() string { return e.Message }

// NotFoundError represents a 404 HTTP error.
type NotFoundError struct {
	Status  int
	Message string
	Body    any
}

func (e *NotFoundError) Error() string { return e.Message }

// ConflictError represents a 409 HTTP error.
type ConflictError struct {
	Status  int
	Message string
	Body    any
}

func (e *ConflictError) Error() string { return e.Message }

// RateLimitedError represents a 429 HTTP error.
type RateLimitedError struct {
	Status  int
	Message string
	Body    any
}

func (e *RateLimitedError) Error() string { return e.Message }

// ApiError represents a generic HTTP error not covered by specific types.
type ApiError struct {
	Status  int
	Message string
	Body    any
}

func (e *ApiError) Error() string { return e.Message }

// TimeoutError represents a polling timeout.
type TimeoutError struct {
	Message   string
	RunID     string
	ElapsedMs int64
}

func (e *TimeoutError) Error() string { return e.Message }

// DagValidationError represents a DAG validation failure.
type DagValidationError struct {
	Message       string
	Cycles        []string
	MissingRefs   []string
	DuplicateRefs []string
}

func (e *DagValidationError) Error() string { return e.Message }

// MapHttpError maps an HTTP status code to the appropriate SDK error type.
func MapHttpError(status int, message string, body any) error {
	if message == "" {
		message = fmt.Sprintf("HTTP %d", status)
	}
	switch status {
	case 401, 403:
		return &UnauthorizedError{Status: status, Message: message, Body: body}
	case 404:
		return &NotFoundError{Status: status, Message: message, Body: body}
	case 409:
		return &ConflictError{Status: status, Message: message, Body: body}
	case 429:
		return &RateLimitedError{Status: status, Message: message, Body: body}
	default:
		return &ApiError{Status: status, Message: message, Body: body}
	}
}
