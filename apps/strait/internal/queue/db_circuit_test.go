package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
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
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.State() != CircuitClosed {
		t.Errorf("state = %v, want closed", c.State())
	}
}

func TestCircuit_OpensAtThreshold(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	if c.State() != CircuitOpen {
		t.Errorf("state = %v, want open", c.State())
	}
	// Next call short-circuits.
	err := c.Do(context.Background(), func(_ context.Context) error {
		t.Fatal("fn should not run when circuit open")
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuit_HalfOpenAfterCooldown(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	boom := errors.New("boom")
	for range 3 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	}
	if c.State() != CircuitOpen {
		t.Fatalf("expected open")
	}
	// Advance past OpenFor.
	now = now.Add(200 * time.Millisecond)
	if c.State() != CircuitHalfOpen {
		t.Errorf("expected half-open after cooldown, got %v", c.State())
	}
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
	if err != nil {
		t.Errorf("probe err = %v", err)
	}
	if c.State() != CircuitClosed {
		t.Errorf("state = %v, want closed", c.State())
	}
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
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("probe err = %v, want context.Canceled", err)
	}
	if c.State() != CircuitHalfOpen {
		t.Fatalf("state = %v, want half-open after canceled probe", c.State())
	}

	err = c.Do(context.Background(), func(_ context.Context) error { return nil })
	if err != nil {
		t.Fatalf("second probe err = %v", err)
	}
	if c.State() != CircuitClosed {
		t.Fatalf("state = %v, want closed after successful retry probe", c.State())
	}
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
				t.Error("concurrent half-open caller should not run")
				return nil
			}); errors.Is(err, ErrCircuitOpen) {
				blocked.Add(1)
			}
		})
	}
	wg.Wait()
	if blocked.Load() != 25 {
		t.Fatalf("blocked probes = %d, want 25", blocked.Load())
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("probe err = %v", err)
	}
	if c.State() != CircuitClosed {
		t.Fatalf("state = %v, want closed", c.State())
	}
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
	if c.State() != CircuitOpen {
		t.Errorf("state after failed probe = %v, want open", c.State())
	}
	// Next cooldown should be 2x initial.
	now = now.Add(150 * time.Millisecond)
	if c.State() != CircuitOpen {
		t.Error("should still be open within doubled cooldown")
	}
	now = now.Add(100 * time.Millisecond)
	if c.State() != CircuitHalfOpen {
		t.Errorf("state = %v, want half-open after doubled cooldown", c.State())
	}
}

func TestCircuit_DoesNotCountContextCanceled(t *testing.T) {
	now := time.Now()
	c := newTestCircuit(&now)
	for range 10 {
		_ = c.Do(context.Background(), func(_ context.Context) error { return context.Canceled })
	}
	if c.State() != CircuitClosed {
		t.Errorf("context.Canceled should not trip breaker, got %v", c.State())
	}
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
	// Only the latest failure is in the window, so breaker stays closed.
	if c.State() != CircuitClosed {
		t.Errorf("expected closed after window pruning, got %v", c.State())
	}
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
	if c.State() != CircuitOpen {
		t.Errorf("state after storm = %v", c.State())
	}
	if count.Load() == 0 {
		t.Error("expected some calls to be short-circuited")
	}
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
	if d := c.currentOpenDuration(); d > 300*time.Millisecond {
		t.Errorf("duration %v exceeds cap", d)
	}
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

	// Closed initially.
	if c.State() != CircuitClosed {
		t.Fatalf("initial state = %v", c.State())
	}

	// Closed -> Open: two consecutive failures trip the breaker.
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	if c.State() != CircuitOpen {
		t.Fatalf("after threshold failures state = %v, want open", c.State())
	}

	// Open -> HalfOpen: advance past OpenFor; next State() observes half-open.
	now = now.Add(100 * time.Millisecond)
	if s := c.State(); s != CircuitHalfOpen {
		t.Fatalf("after open timeout state = %v, want half_open", s)
	}

	// HalfOpen -> Open: a probe failure re-opens.
	_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
	if c.State() != CircuitOpen {
		t.Fatalf("after probe failure state = %v, want open", c.State())
	}

	// Open -> HalfOpen -> Closed: advance past OpenFor and run a success probe.
	now = now.Add(500 * time.Millisecond)
	if s := c.State(); s != CircuitHalfOpen {
		t.Fatalf("second half-open observation state = %v", s)
	}
	if err := c.Do(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("probe success err = %v", err)
	}
	if c.State() != CircuitClosed {
		t.Fatalf("after success probe state = %v, want closed", c.State())
	}
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
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
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
