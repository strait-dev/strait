package worker

import (
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})
	if cb.State() != circuitClosed {
		t.Fatalf("initial state = %q, want %q", cb.State(), circuitClosed)
	}
	if !cb.Allow() {
		t.Fatal("Allow() = false in closed state, want true")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != circuitClosed {
		t.Fatalf("state after 2 failures = %q, want %q", cb.State(), circuitClosed)
	}

	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("state after 3 failures = %q, want %q", cb.State(), circuitOpen)
	}
	if cb.Allow() {
		t.Fatal("Allow() = true in open state, want false")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Minute})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	if cb.ConsecutiveFailures() != 0 {
		t.Fatalf("consecutive failures after success = %d, want 0", cb.ConsecutiveFailures())
	}

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != circuitClosed {
		t.Fatalf("state after reset+2 = %q, want %q", cb.State(), circuitClosed)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterCooldown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	cb.now = func() time.Time { return now }

	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("state = %q, want %q", cb.State(), circuitOpen)
	}

	cb.now = func() time.Time { return now.Add(30 * time.Second) }
	if cb.Allow() {
		t.Fatal("Allow() = true before cooldown elapsed, want false")
	}

	cb.now = func() time.Time { return now.Add(61 * time.Second) }
	if !cb.Allow() {
		t.Fatal("Allow() = false after cooldown elapsed, want true")
	}
	if cb.State() != circuitHalfOpen {
		t.Fatalf("state = %q, want %q", cb.State(), circuitHalfOpen)
	}
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
	if cb.State() != circuitClosed {
		t.Fatalf("state after half-open success = %q, want %q", cb.State(), circuitClosed)
	}
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
	if cb.State() != circuitOpen {
		t.Fatalf("state after half-open failure = %q, want %q", cb.State(), circuitOpen)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	cb.RecordFailure()
	if cb.State() != circuitOpen {
		t.Fatalf("state = %q, want %q", cb.State(), circuitOpen)
	}

	cb.Reset()
	if cb.State() != circuitClosed {
		t.Fatalf("state after reset = %q, want %q", cb.State(), circuitClosed)
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Fatalf("failures after reset = %d, want 0", cb.ConsecutiveFailures())
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.threshold != defaultCircuitFailureThreshold {
		t.Fatalf("threshold = %d, want %d", cb.threshold, defaultCircuitFailureThreshold)
	}
	if cb.openDuration != defaultCircuitOpenDuration {
		t.Fatalf("openDuration = %v, want %v", cb.openDuration, defaultCircuitOpenDuration)
	}
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
	if state != circuitClosed && state != circuitOpen && state != circuitHalfOpen {
		t.Fatalf("invalid state %q after concurrent access", state)
	}
}
