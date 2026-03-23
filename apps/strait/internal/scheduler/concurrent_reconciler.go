package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// ConcurrentReconciler periodically reconciles Redis concurrent run counters
// with the actual count of executing runs from the database. It also checks
// daily run counter drift for observability.
type ConcurrentReconciler struct {
	enforcer     *billing.Enforcer
	counter      billing.ExecutingRunCounter
	dailyCounter billing.DailyRunCounter
	interval     time.Duration
}

// NewConcurrentReconciler creates a new reconciler.
func NewConcurrentReconciler(enforcer *billing.Enforcer, counter billing.ExecutingRunCounter, interval time.Duration) *ConcurrentReconciler {
	return &ConcurrentReconciler{
		enforcer: enforcer,
		counter:  counter,
		interval: interval,
	}
}

// WithDailyRunCounter enables daily run counter drift detection alongside
// the concurrent reconciliation loop.
func (r *ConcurrentReconciler) WithDailyRunCounter(counter billing.DailyRunCounter) *ConcurrentReconciler {
	r.dailyCounter = counter
	return r
}

// Run starts the periodic reconciliation loop.
func (r *ConcurrentReconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.enforcer.ReconcileAllConcurrentCounts(ctx, r.counter); err != nil {
				slog.Warn("concurrent run reconciliation failed", "error", err)
			}
			if r.dailyCounter != nil {
				if err := r.enforcer.ReconcileDailyRunCounts(ctx, r.dailyCounter); err != nil {
					slog.Warn("daily run counter reconciliation failed", "error", err)
				}
			}
		}
	}
}
