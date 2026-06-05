package worker

import (
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})
	require.Equal(t,
		circuitClosed,

		cb.State())
	require.True(t,
		cb.Allow())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})

	cb.RecordFailure()
	cb.RecordFailure()
	require.Equal(t,
		circuitClosed,

		cb.State())

	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,

		cb.State())
	require.False(t,
		cb.Allow())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	require.Equal(t, 0, cb.ConsecutiveFailures())

	cb.RecordFailure()
	cb.RecordFailure()
	require.Equal(t,
		circuitClosed,

		cb.State())
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterCooldown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	cb.now = func() time.Time { return now }

	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,

		cb.State())

	cb.now = func() time.Time { return now.Add(30 * time.Second) }
	require.False(t,
		cb.Allow())

	cb.now = func() time.Time { return now.Add(61 * time.Second) }
	require.True(t,
		cb.Allow())
	require.Equal(t,
		circuitHalfOpen,

		cb.State())
}

func TestCircuitBreaker_HalfOpenClosesOnSuccess(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Second})
	cb.now = func() time.Time { return now }

	cb.RecordFailure()
	cb.now = func() time.Time { return now.Add(2 * time.Second) }
	cb.Allow()

	cb.RecordSuccess()
	require.Equal(t,
		circuitClosed,

		cb.State())
}

func TestCircuitBreaker_HalfOpenReopensOnFailure(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Second})
	cb.now = func() time.Time { return now }

	cb.RecordFailure()
	cb.now = func() time.Time { return now.Add(2 * time.Second) }
	cb.Allow()

	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,

		cb.State())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	cb.RecordFailure()
	require.Equal(t,
		circuitOpen,

		cb.State())

	cb.Reset()
	require.Equal(t,
		circuitClosed,

		cb.State())
	require.Equal(t, 0, cb.ConsecutiveFailures())
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	require.Equal(t,
		defaultCircuitFailureThreshold,

		cb.threshold,
	)
	require.Equal(t,
		defaultCircuitOpenDuration,

		cb.openDuration,
	)
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 100, OpenDuration: time.Minute})

	var wg conc.WaitGroup
	for range 50 {
		wg.Go(func() {
			cb.Allow()
		})
		wg.Go(func() {
			cb.RecordFailure()
		})
		wg.Go(func() {
			cb.RecordSuccess()
		})
	}
	wg.Wait()

	state := cb.State()
	require.False(t,
		state !=

			circuitClosed && state != circuitOpen &&
			state != circuitHalfOpen)
}
