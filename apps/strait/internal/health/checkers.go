package health

import (
	"context"
	"fmt"
	"time"
)

// CriticalChecker wraps a Checker and marks it as critical or non-critical.
// When a non-critical checker fails, the registry reports "degraded" instead of "down".
type CriticalChecker struct {
	Checker
	Critical bool
}

// IsCritical returns whether the checker is critical (causes "down" on failure).
func IsCritical(c Checker) bool {
	if cc, ok := c.(*CriticalChecker); ok {
		return cc.Critical
	}
	return true // default: all checkers are critical
}

// NewCriticalChecker wraps a check function with a criticality flag.
func NewCriticalChecker(name string, critical bool, fn func(ctx context.Context) error) Checker {
	return &CriticalChecker{
		Checker:  NewChecker(name, fn),
		Critical: critical,
	}
}

type PoolStats interface {
	Available() int
	ActiveCount() int
}

func NewPoolChecker(pool PoolStats) Checker {
	return NewChecker("worker_pool", func(_ context.Context) error {
		if pool.Available() <= 0 && pool.ActiveCount() > 0 {
			return fmt.Errorf("worker pool exhausted: %d active, 0 available", pool.ActiveCount())
		}
		return nil
	})
}

func NewMigrationChecker(current uint, dirty bool, err error) Checker {
	return NewChecker("migrations", func(_ context.Context) error {
		if err != nil {
			return fmt.Errorf("migration status unknown: %w", err)
		}
		if dirty {
			return fmt.Errorf("migration %d is dirty", current)
		}
		return nil
	})
}

func NewSchedulerChecker(lastTickFn func() time.Time, maxAge time.Duration) Checker {
	return NewChecker("scheduler", func(_ context.Context) error {
		lastTick := lastTickFn()
		if lastTick.IsZero() {
			return fmt.Errorf("scheduler tick unavailable")
		}
		if time.Since(lastTick) > maxAge {
			return fmt.Errorf("scheduler stale: last tick at %s exceeds max age %s", lastTick.UTC().Format(time.RFC3339), maxAge)
		}
		return nil
	})
}

func NewQueueDepthChecker(depthFn func(ctx context.Context) (int64, error), threshold int64) Checker {
	return NewChecker("queue_depth", func(ctx context.Context) error {
		depth, err := depthFn(ctx)
		if err != nil {
			return fmt.Errorf("queue depth check failed: %w", err)
		}
		if depth > threshold {
			return fmt.Errorf("queue depth %d exceeds threshold %d", depth, threshold)
		}
		return nil
	})
}
