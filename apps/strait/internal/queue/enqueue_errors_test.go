package queue

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalEnqueueError_Error(t *testing.T) {
	baseErr := errors.New("duplicate idempotency key")

	tests := []struct {
		name string
		err  *TerminalEnqueueError
		want string
	}{
		{
			name: "nil receiver",
			err:  nil,
			want: ErrEnqueueTerminal.Error(),
		},
		{
			name: "without reason",
			err: &TerminalEnqueueError{
				Err: baseErr,
			},
			want: "enqueue terminal failure: duplicate idempotency key",
		},
		{
			name: "with reason",
			err: &TerminalEnqueueError{
				Reason: "invalid payload",
				Err:    baseErr,
			},
			want: "enqueue terminal failure (invalid payload): duplicate idempotency key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}

func TestTerminalEnqueueError_Unwrap(t *testing.T) {
	var nilTerminal *TerminalEnqueueError
	require.ErrorIs(t, nilTerminal.Unwrap(), ErrEnqueueTerminal)

	withoutCause := &TerminalEnqueueError{Reason: "invalid payload"}
	require.ErrorIs(t, withoutCause.Unwrap(), ErrEnqueueTerminal)

	cause := errors.New("json marshal failed")
	withCause := &TerminalEnqueueError{
		Reason: "invalid payload",
		Err:    cause,
	}
	require.ErrorIs(t, withCause.Unwrap(), ErrEnqueueTerminal)
	require.ErrorIs(t, withCause.Unwrap(), cause)
}

func TestAsTerminalEnqueue(t *testing.T) {
	terminal := &TerminalEnqueueError{
		Reason: "invalid payload",
		Err:    errors.New("json marshal failed"),
	}

	got, ok := AsTerminalEnqueue(terminal)
	require.True(t, ok)
	assert.Same(t, terminal, got)

	wrapped := errors.Join(errors.New("outer"), terminal)
	got, ok = AsTerminalEnqueue(wrapped)
	require.True(t, ok)
	assert.Same(t, terminal, got)

	got, ok = AsTerminalEnqueue(errors.New("transient"))
	assert.False(t, ok)
	assert.Nil(t, got)
}
