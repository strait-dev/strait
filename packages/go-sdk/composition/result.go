// Package composition provides composition helpers for the Strait SDK
// including Result types, retry logic, polling, pagination, and
// deployment workflow helpers.
package composition

// Result represents the outcome of an operation that may fail.
type Result[T any] struct {
	ok     bool
	output T
	err    error
}

// Ok creates a successful Result.
func Ok[T any](value T) Result[T] {
	return Result[T]{ok: true, output: value}
}

// Err creates a failed Result.
func Err[T any](err error) Result[T] {
	return Result[T]{ok: false, err: err}
}

// IsOk returns true if the result is successful.
func (r Result[T]) IsOk() bool { return r.ok }

// IsErr returns true if the result is a failure.
func (r Result[T]) IsErr() bool { return !r.ok }

// Unwrap returns the value or panics if the result is an error.
func (r Result[T]) Unwrap() T {
	if !r.ok {
		panic(r.err)
	}
	return r.output
}

// UnwrapErr returns the value and error separately.
func (r Result[T]) UnwrapErr() (T, error) {
	return r.output, r.err
}

// Match executes the appropriate function based on the result.
func (r Result[T]) Match(onOk func(T), onErr func(error)) {
	if r.ok {
		onOk(r.output)
	} else {
		onErr(r.err)
	}
}

// FromFunc executes a function and wraps the outcome in a Result.
func FromFunc[T any](fn func() (T, error)) Result[T] {
	val, err := fn()
	if err != nil {
		return Err[T](err)
	}
	return Ok(val)
}
