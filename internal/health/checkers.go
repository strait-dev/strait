package health

import (
	"context"
	"fmt"
)

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
