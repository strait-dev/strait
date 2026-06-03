package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// ConcurrentReconciler periodically reconciles Redis concurrent run counters
// with the actual count of executing runs from the database.
type ConcurrentReconciler struct {
	enforcer *billing.Enforcer
	counter  billing.ExecutingRunCounter
	interval time.Duration
}

// NewConcurrentReconciler creates a new reconciler.
func NewConcurrentReconciler(enforcer *billing.Enforcer, counter billing.ExecutingRunCounter, interval time.Duration) *ConcurrentReconciler {
	return &ConcurrentReconciler{
		enforcer: enforcer,
		counter:  counter,
		interval: interval,
	}
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
			runSchedulerCycleCheckIn(ctx, r.interval, func() {
				if err := r.enforcer.ReconcileAllConcurrentCounts(ctx, r.counter); err != nil {
					slog.Warn("concurrent run reconciliation failed", "error", err)
				}
			})
		}
	}
}
