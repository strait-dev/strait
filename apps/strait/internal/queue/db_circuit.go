package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DB circuit breaker for the queue hot path.
//
// When Postgres is slow (not down -- slow), the executor keeps submitting
// claim queries faster than the DB can drain them. Goroutines pile up in
// the pgxpool wait queue until the process OOMs. An open-circuit wrapper
// lets the executor shed load before that cascade.
//
// States:
//   - Closed:    normal operation. Failures increment a counter.
//                At failureThreshold within failureWindow the circuit opens.
//   - Open:      all calls return ErrCircuitOpen immediately. A timer fires
//                after openFor to move the circuit to half-open.
//   - Half-open: a single probe is allowed through. Success closes the
//                circuit; failure re-opens with exponential backoff.
//
// The breaker counts errors from the wrapped function's return. It does
// NOT count context.Canceled -- cancellation is a caller-side decision
// and we don't want a cancelling request to force the breaker open.

// DBCircuitState is the public state of the breaker.
type DBCircuitState int

const (
	CircuitClosed DBCircuitState = iota
	CircuitHalfOpen
	CircuitOpen
)

func (s DBCircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitHalfOpen:
		return "half_open"
	case CircuitOpen:
		return "open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned by Do when the circuit is open.
var ErrCircuitOpen = errors.New("db circuit: open")

// DBCircuitConfig configures the breaker.
type DBCircuitConfig struct {
	FailureThreshold int           // consecutive failures before open
	FailureWindow    time.Duration // window for counting failures
	OpenFor          time.Duration // initial open duration; grows exponentially
	MaxOpenFor       time.Duration // cap on the exponential growth
	Clock            func() time.Time
	Logger           *slog.Logger
}

func defaultCircuitConfig() DBCircuitConfig {
	return DBCircuitConfig{
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		OpenFor:          2 * time.Second,
		MaxOpenFor:       60 * time.Second,
		Clock:            time.Now,
		Logger:           slog.Default(),
	}
}

// DBCircuit is a concurrency-safe circuit breaker.
type DBCircuit struct {
	cfg     DBCircuitConfig
	mu      sync.Mutex
	state   DBCircuitState
	openAt  time.Time
	fails   []time.Time
	attempt int // number of consecutive opens, drives exponential backoff

	probeInFlight bool // true while the single half-open probe is running
}

// NewDBCircuit builds a circuit with the given config. Zero-valued fields
// fall back to sensible defaults.
func NewDBCircuit(cfg DBCircuitConfig) *DBCircuit {
	d := defaultCircuitConfig()
	if cfg.FailureThreshold > 0 {
		d.FailureThreshold = cfg.FailureThreshold
	}
	if cfg.FailureWindow > 0 {
		d.FailureWindow = cfg.FailureWindow
	}
	if cfg.OpenFor > 0 {
		d.OpenFor = cfg.OpenFor
	}
	if cfg.MaxOpenFor > 0 {
		d.MaxOpenFor = cfg.MaxOpenFor
	}
	if cfg.Clock != nil {
		d.Clock = cfg.Clock
	}
	if cfg.Logger != nil {
		d.Logger = cfg.Logger
	}
	return &DBCircuit{cfg: d, state: CircuitClosed}
}

// recordTransitionLocked emits a transition counter for the given from→to
// change. Caller must hold c.mu. A no-op when the singleton queue metrics
// have not been initialised (tests without an OTEL provider).
func (c *DBCircuit) recordTransitionLocked(from, to DBCircuitState) {
	if from == to {
		return
	}
	c.state = to
	qm, err := Metrics()
	if err != nil {
		return
	}
	if qm == nil || qm.CircuitStateTransitions == nil {
		return
	}
	qm.CircuitStateTransitions.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("from", from.String()),
			attribute.String("to", to.String()),
		),
	)
}

// State returns the current circuit state. Concurrency-safe.
func (c *DBCircuit) State() DBCircuitState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stateLocked()
}

// stateLocked is State without the mutex (caller holds it).
func (c *DBCircuit) stateLocked() DBCircuitState {
	if c.state == CircuitOpen && c.cfg.Clock().After(c.openAt.Add(c.currentOpenDuration())) {
		c.recordTransitionLocked(CircuitOpen, CircuitHalfOpen)
	}
	return c.state
}

// Do wraps a call through the circuit. If the circuit is open, returns
// ErrCircuitOpen immediately without invoking fn. If half-open, fn is
// invoked as a probe: success → closed, failure → open with exponential
// backoff. If closed, fn runs normally and its outcome feeds the failure
// counter.
func (c *DBCircuit) Do(ctx context.Context, fn func(context.Context) error) error {
	c.mu.Lock()
	switch c.stateLocked() {
	case CircuitOpen:
		c.mu.Unlock()
		return ErrCircuitOpen
	case CircuitHalfOpen:
		if c.probeInFlight {
			c.mu.Unlock()
			return ErrCircuitOpen
		}
		c.probeInFlight = true
		c.mu.Unlock()
		if err := fn(ctx); err != nil {
			c.recordFailure(err, true)
			return err
		}
		c.recordSuccess()
		return nil
	case CircuitClosed:
		c.mu.Unlock()
		err := fn(ctx)
		if err != nil {
			c.recordFailure(err, false)
		}
		return err
	}
	c.mu.Unlock()
	return nil
}

// recordSuccess transitions half-open → closed, resets counters.
func (c *DBCircuit) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == CircuitHalfOpen || c.state == CircuitClosed {
		prev := c.state
		c.recordTransitionLocked(prev, CircuitClosed)
		c.fails = c.fails[:0]
		c.attempt = 0
		c.probeInFlight = false
	}
}

// recordFailure notes an error. During context.Canceled we skip counting.
// halfOpen=true means the failure came from a probe; that immediately
// re-opens the circuit regardless of the window.
func (c *DBCircuit) recordFailure(err error, halfOpen bool) {
	if errors.Is(err, context.Canceled) {
		if halfOpen {
			c.mu.Lock()
			if c.state == CircuitHalfOpen && c.probeInFlight {
				c.probeInFlight = false
			}
			c.mu.Unlock()
		}
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.cfg.Clock()
	if halfOpen {
		if c.state != CircuitHalfOpen || !c.probeInFlight {
			return
		}
		c.probeInFlight = false
		c.attempt++
		c.recordTransitionLocked(CircuitHalfOpen, CircuitOpen)
		c.openAt = now
		c.cfg.Logger.Warn("db circuit: re-opened after half-open probe failure",
			"attempt", c.attempt, "open_for", c.currentOpenDuration(),
		)
		return
	}
	if c.state != CircuitClosed {
		return
	}
	// Prune failures outside the window.
	cutoff := now.Add(-c.cfg.FailureWindow)
	kept := c.fails[:0]
	for _, t := range c.fails {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	c.fails = kept
	if len(c.fails) >= c.cfg.FailureThreshold {
		c.recordTransitionLocked(CircuitClosed, CircuitOpen)
		c.openAt = now
		c.attempt = 1
		c.cfg.Logger.Warn("db circuit: opened",
			"failures_in_window", len(c.fails),
			"window", c.cfg.FailureWindow,
			"open_for", c.currentOpenDuration(),
		)
	}
}

// currentOpenDuration applies exponential backoff to OpenFor based on the
// attempt count, capped at MaxOpenFor.
func (c *DBCircuit) currentOpenDuration() time.Duration {
	d := c.cfg.OpenFor
	for i := 1; i < c.attempt; i++ {
		d *= 2
		if d >= c.cfg.MaxOpenFor {
			return c.cfg.MaxOpenFor
		}
	}
	return d
}
