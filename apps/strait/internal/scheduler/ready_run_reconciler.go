package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ReadyRunRepairer repairs ready-run delivery for queue backends whose
// claimability is represented by an external ready-event log.
type ReadyRunRepairer interface {
	ReconcileReadyRuns(ctx context.Context, limit int) (int64, error)
}

// ReadyRunReconciler periodically re-emits missing ready events for PgQue.
type ReadyRunReconciler struct {
	interval  time.Duration
	logger    *slog.Logger
	repairer  ReadyRunRepairer
	repairMax int
}

// NewReadyRunReconciler creates a reconciler; zero interval defaults to 5m.
func NewReadyRunReconciler(repairer ReadyRunRepairer, interval time.Duration, repairMax int) *ReadyRunReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if repairMax <= 0 {
		repairMax = 1000
	}
	return &ReadyRunReconciler{
		interval:  interval,
		logger:    slog.Default(),
		repairer:  repairer,
		repairMax: repairMax,
	}
}

func (r *ReadyRunReconciler) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("ready-run-reconciler", r.interval, r.logger, func(loopCtx context.Context) {
		if err := r.reconcileOnce(loopCtx); err != nil {
			r.logger.Error("ready run reconciler failed", "error", err)
		}
	})
	loop.Run(ctx)
}

func (r *ReadyRunReconciler) reconcileOnce(ctx context.Context) error {
	if r.repairer == nil {
		return nil
	}
	repaired, err := r.repairer.ReconcileReadyRuns(ctx, r.repairMax)
	if err != nil {
		return fmt.Errorf("reconcile pgque ready runs: %w", err)
	}
	if repaired > 0 {
		r.logger.Warn("ready run reconciler: re-emitted pgque ready runs", "count", repaired)
	}
	return nil
}
