package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThrottledError_Unwrap(t *testing.T) {
	e := &ThrottledError{ProjectID: "p", RetryAfter: 0}
	assert.ErrorIs(t,
		e, ErrEnqueueThrottled)
}

func TestAsThrottled_Positive(t *testing.T) {
	e := &ThrottledError{ProjectID: "p"}
	if tthrot, ok := AsThrottled(e); !ok || tthrot.ProjectID != "p" {
		assert.Failf(t, "test failure",

			"AsThrottled failed: %+v %v", tthrot, ok)
	}
}

func TestAsThrottled_Negative(t *testing.T) {
	if _, ok := AsThrottled(errors.New("other")); ok {
		assert.Fail(t,

			"non-throttled should not match")
	}
}

func TestBackpressure_NilSafeAndDisabled(t *testing.T) {
	var b *Backpressure
	require.NoError(
		t, b.TryConsume(context.
			Background(), "p"))

	b2 := NewBackpressure(nil, BackpressureConfig{}, false)
	require.NoError(
		t, b2.TryConsume(context.
			Background(), "p"))
}

func TestBackpressure_EmptyProjectID(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	require.NoError(
		t, b.TryConsume(context.
			Background(), ""))
}

func TestBackpressure_DefaultConfig(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	assert.False(t,
		b.cfg.DefaultMaxTokens !=
			1000 || b.cfg.DefaultRefillPerSec != 100)
}
