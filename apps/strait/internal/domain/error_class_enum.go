package domain

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// Round 2 Phase 1: typed ErrorClass.
//
// ErrorClass was originally a set of untyped string constants in types.go.
// To keep backwards compatibility with the large number of call sites that
// assign `run.ErrorClass = domain.ErrorClassUnknown` via the string-typed
// field, we introduce a companion ErrorClassEnum type here with Scan/Value
// and helper predicates. New code at trust boundaries should use
// ParseErrorClass to reject unknown values.

// ErrorClassEnum is the typed form of the ErrorClass* untyped string
// constants in types.go. Use the existing string constants at assignment
// sites and convert via ErrorClassEnum(...) when you need typed behavior.
type ErrorClassEnum string

// Scan / Value for pgx integration.
func (e *ErrorClassEnum) Scan(src any) error {
	if src == nil {
		*e = ""
		return nil
	}
	switch v := src.(type) {
	case string:
		*e = ErrorClassEnum(v)
	case []byte:
		*e = ErrorClassEnum(v)
	default:
		return fmt.Errorf("ErrorClassEnum.Scan: unsupported type %T", src)
	}
	if *e != "" && !e.IsValid() {
		return fmt.Errorf("ErrorClassEnum.Scan: unknown error class %q", string(*e))
	}
	return nil
}

func (e ErrorClassEnum) Value() (driver.Value, error) {
	if e == "" {
		return nil, nil
	}
	if !e.IsValid() {
		return nil, fmt.Errorf("ErrorClassEnum.Value: invalid %q", string(e))
	}
	return string(e), nil
}

// IsValid reports whether the enum is one of the known classes.
func (e ErrorClassEnum) IsValid() bool {
	return ValidErrorClasses[string(e)]
}

// IsRetryable encodes the retry policy: client / auth / budget / oom
// terminate; everything else retries.
func (e ErrorClassEnum) IsRetryable() bool {
	switch string(e) {
	case ErrorClassClient, ErrorClassAuth, ErrorClassBudget, ErrorClassOOM:
		return false
	default:
		return true
	}
}

// IsTransient identifies classes where a short retry is most likely to
// succeed: rate_limited, transient, connection, timeout.
func (e ErrorClassEnum) IsTransient() bool {
	switch string(e) {
	case ErrorClassRateLimited, ErrorClassTransient, ErrorClassConnection, ErrorClassTimeout:
		return true
	default:
		return false
	}
}

// ParseErrorClass validates and returns an ErrorClassEnum. Used at trust
// boundaries to reject unknown values.
func ParseErrorClass(raw string) (ErrorClassEnum, error) {
	if raw == "" {
		return "", nil
	}
	e := ErrorClassEnum(raw)
	if !e.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownErrorClass, raw)
	}
	return e, nil
}

// ErrUnknownErrorClass is returned by ParseErrorClass for values outside
// the closed set.
var ErrUnknownErrorClass = errors.New("unknown error class")
