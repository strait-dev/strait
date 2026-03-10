package worker

import (
	"sync"
	"time"
)

// CircuitBreaker implements a thread-safe circuit breaker for webhook delivery.
//
// State machine:
//
//	Closed → Open:     consecutive failures >= threshold
//	Open → HalfOpen:   after openDuration elapses
//	HalfOpen → Closed: a single success
//	HalfOpen → Open:   any failure
type CircuitBreaker struct {
	mu                  sync.RWMutex
	state               string
	consecutiveFailures int
	threshold           int
	openDuration        time.Duration
	openedAt            time.Time
	now                 func() time.Time
}

const (
	circuitClosed   = "closed"
	circuitOpen     = "open"
	circuitHalfOpen = "half_open"
)

type CircuitBreakerConfig struct {
	FailureThreshold int
	OpenDuration     time.Duration
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = defaultCircuitFailureThreshold
	}
	openDuration := cfg.OpenDuration
	if openDuration <= 0 {
		openDuration = defaultCircuitOpenDuration
	}
	return &CircuitBreaker{
		state:        circuitClosed,
		threshold:    threshold,
		openDuration: openDuration,
		now:          time.Now,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return true
	case circuitOpen:
		if cb.now().Sub(cb.openedAt) >= cb.openDuration {
			cb.state = circuitHalfOpen
			return true
		}
		return false
	case circuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFailures = 0
	if cb.state == circuitHalfOpen {
		cb.state = circuitClosed
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFailures++

	if cb.state == circuitHalfOpen {
		cb.state = circuitOpen
		cb.openedAt = cb.now()
		return
	}

	if cb.consecutiveFailures >= cb.threshold {
		cb.state = circuitOpen
		cb.openedAt = cb.now()
	}
}

func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) ConsecutiveFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveFailures
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = circuitClosed
	cb.consecutiveFailures = 0
	cb.openedAt = time.Time{}
}
