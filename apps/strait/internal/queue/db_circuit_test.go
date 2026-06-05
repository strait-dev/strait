package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the DB circuit breaker.

func newTestCircuit(now *time.Time) *DBCircuit {
	return NewDBCircuit(DBCircuitConfig{
		FailureThreshold: 3,
		FailureWindow:    10 * time.Second,
		OpenFor:          100 * time.Millisecond,
		MaxOpenFor:       1 * time.Second,
		Clock:            func() time.Time { return *now },
	})
}

func TestCircuit_ClosedPassesThrough(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	err := c.Do(context.Background(), func(_ context.Context) error { return nil })
	require.NoError(t, err)
	assert.Equal(t,
		CircuitClosed,
		c.
			State())
}

func TestCircuit_OpensAtThreshold(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	assert.Equal(t,
		CircuitOpen,
		c.State())

	// Next call short-circuits.
	err := c.Do(context.Background(), func(_ context.Context) error {
		require.Fail(t,

			"fn should not run when circuit open")
		return nil
	})
	assert.ErrorIs(t,
		err, ErrCircuitOpen)
}

func TestCircuit_HalfOpenAfterCooldown(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	require.Equal(t,
		CircuitOpen,
		c.State())

	// Advance past OpenFor.
	now = now.Add(200 * time.Millisecond)
	assert.Equal(t,
		CircuitHalfOpen,

		c.State())
}

func TestCircuit_HalfOpenSuccessCloses(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	now = now.Add(200 * time.Millisecond)

	err := c.Do(context.Background(), func(_ context.Context) error { return nil })
	require.NoError(
		t, err)
	assert.Equal(t,
		CircuitClosed,
		c.
			State())
}

func TestCircuit_HalfOpenCanceledProbeAllowsRetry(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	now = now.Add(200 * time.Millisecond)

	err := c.Do(context.Background(), func(_ context.Context) error { return context.Canceled })
	require.ErrorIs(t,
		err, context.
			Canceled,
	)
	require.Equal(t,
		CircuitHalfOpen,

		c.State())

	err = c.Do(context.Background(), func(_ context.Context) error { return nil })
	require.NoError(t, err)
	require.Equal(t,
		CircuitClosed,
		c.
			State())
}

func TestCircuit_HalfOpenAllowsOnlyOneProbe(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	now := time.Now()
	c := NewDBCircuit(DBCircuitConfig{
		FailureThreshold: 1,
		FailureWindow:    time.Second,
		OpenFor:          time.Millisecond,
		MaxOpenFor:       time.Second,
		Clock:            func() time.Time { return now },
	})
	boom := errors.New("boom")
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	now = now.Add(2 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	concWG.Go(func() {
		done <- c.Do(context.Background(), func(_ context.Context) error {
			close(started)
			<-release
			return nil
		})
	})
	<-started

	var blocked atomic.Int64
	var wg sync.WaitGroup
	for range 25 {
		wg.Go(func() {
			if err := c.Do(context.Background(), func(_ context.Context) error {
				assert.Fail(t,

					"concurrent half-open caller should not run")
				return nil
			}); errors.Is(err, ErrCircuitOpen) {
				blocked.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 25, blocked.
		Load())

	close(release)
	require.NoError(t, <-done)
	require.Equal(t,
		CircuitClosed,
		c.
			State())
}

func TestCircuit_HalfOpenFailureReopensExponentially(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	// First cooldown.
	now = now.Add(200 * time.Millisecond)
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	assert.Equal(t,
		CircuitOpen,
		c.State())

	// Next cooldown should be 2x initial.
	now = now.Add(150 * time.Millisecond)
	assert.Equal(t,
		CircuitOpen,
		c.State())

	now = now.Add(100 * time.Millisecond)
	assert.Equal(t,
		CircuitHalfOpen,

		c.State())
}

func TestCircuit_DoesNotCountContextCanceled(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	for range 10 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return context.Canceled })
	}
	assert.Equal(t,
		CircuitClosed,
		c.
			State())
}

func TestCircuit_WindowPruning(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	// Advance past the window; old failures should be pruned.
	now = now.Add(11 * time.Second)
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	assert.Equal(t,
		CircuitClosed,
		c.
			State())

	// Only the latest failure is in the window, so breaker stays closed.
}

func TestCircuit_ConcurrentFailuresOpenOnce(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	var wg sync.WaitGroup
	var count atomic.Int64
	for range 100 {
		wg.Go(func() {
			if err := c.Do(context.Background(), func(_ context.Context) error { return boom }); errors.Is(err, ErrCircuitOpen) {
				count.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t,
		CircuitOpen,
		c.State())
	assert.NotEqual(t, 0, count.
		Load())
}

func TestCircuit_MaxOpenForCap(t *testing.T) {
	now := time.Now()
	c := NewDBCircuit(DBCircuitConfig{
		FailureThreshold: 1,
		FailureWindow:    time.Second,
		OpenFor:          100 * time.Millisecond,
		MaxOpenFor:       300 * time.Millisecond,
		Clock:            func() time.Time { return now },
	})
	boom := errors.New("boom")
	// Trip and re-trip many times to push exponential duration past the cap.
	for range 10 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
		now = now.Add(400 * time.Millisecond)
	}
	assert.LessOrEqual(t, c.currentOpenDuration(), 300*
		time.Millisecond,
	)
}

// TestCircuit_AllTransitionsVisible walks the four transition edges
// (Closed→Open, Open→HalfOpen, HalfOpen→Open, HalfOpen→Closed) and
// asserts that the breaker's observed state matches at each step. The
// recordTransitionLocked hook that emits the telemetry counter is
// exercised on every transition here.
func TestCircuit_AllTransitionsVisible(t *testing.T) {
	now := time.Now()
	c := NewDBCircuit(DBCircuitConfig{
		FailureThreshold: 2,
		FailureWindow:    time.Second,
		OpenFor:          50 * time.Millisecond,
		MaxOpenFor:       time.Second,
		Clock:            func() time.Time { return now },
	})
	boom := errors.New("boom")
	require.Equal(t,
		CircuitClosed,
		c.
			State())

	// Closed initially.

	// Closed -> Open: two consecutive failures trip the breaker.
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	require.Equal(t,
		CircuitOpen,
		c.State())

	// Open -> HalfOpen: advance past OpenFor; next State() observes half-open.
	now = now.Add(100 * time.Millisecond)
	require.Equal(t,
		CircuitHalfOpen,

		c.State())

	// HalfOpen -> Open: a probe failure re-opens.
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	require.Equal(t,
		CircuitOpen,
		c.State())

	// Open -> HalfOpen -> Closed: advance past OpenFor and run a success probe.
	now = now.Add(500 * time.Millisecond)
	require.Equal(t,
		CircuitHalfOpen,

		c.State())
	require.NoError(t, c.Do(context.
		Background(), func(_ context.
		Context) error {
		return nil
	}))
	require.Equal(t,
		CircuitClosed,
		c.
			State())
}

func FuzzCircuitTransitions(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(0), uint8(5))
	f.Fuzz(func(t *testing.T, pattern, n uint8) {
		now := time.Now()
		c := NewDBCircuit(DBCircuitConfig{
			FailureThreshold: 3,
			FailureWindow:    time.Hour,
			OpenFor:          time.Millisecond,
			MaxOpenFor:       time.Second,
			Clock:            func() time.Time { return now },
		})
		defer func() {
			require.Nil(t, recover())
		}()
		boom := errors.New("boom")
		// Execute n ops with pattern bits selecting success/failure.
		for i := uint8(0); i < n && i < 16; i++ {
			if pattern&(1<<(i%8)) != 0 {
				_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
			} else {
				_ = c.Do(context.Background(), func(_ context.Context) error { return nil })
			}
		}
		_ = c.State()
	})
}
