package queue

import (
	"errors"
	"fmt"
)

// ErrEnqueueTerminal marks enqueue failures that should not be retried by
// outbox promotion or other bounded retry loops.
var ErrEnqueueTerminal = errors.New("enqueue terminal failure")

// TerminalEnqueueError wraps a permanent enqueue error with a stable reason.
type TerminalEnqueueError struct {
	Reason string
	Err    error
}

func (e *TerminalEnqueueError) Error() string {
	if e == nil {
		return ErrEnqueueTerminal.Error()
	}
	if e.Reason == "" {
		return fmt.Sprintf("%v: %v", ErrEnqueueTerminal, e.Err)
	}
	return fmt.Sprintf("%v (%s): %v", ErrEnqueueTerminal, e.Reason, e.Err)
}

func (e *TerminalEnqueueError) Unwrap() error {
	if e == nil || e.Err == nil {
		return ErrEnqueueTerminal
	}
	return errors.Join(ErrEnqueueTerminal, e.Err)
}

// AsTerminalEnqueue returns the wrapped terminal enqueue error when present.
func AsTerminalEnqueue(err error) (*TerminalEnqueueError, bool) {
	var terminal *TerminalEnqueueError
	if errors.As(err, &terminal) {
		return terminal, true
	}
	return nil, false
}
