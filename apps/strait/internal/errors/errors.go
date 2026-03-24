package errors

import (
	"github.com/samber/oops"
)

// Wrap wraps an error with a message and captures a stack trace.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, "%s", msg)
}

// Wrapf wraps an error with a formatted message and captures a stack trace.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, format, args...)
}

// New creates a new error with a stack trace.
func New(msg string) error {
	return oops.Errorf("%s", msg)
}

// In returns a builder scoped to the given component for structured error context.
func In(component string) oops.OopsErrorBuilder {
	return oops.In(component)
}
