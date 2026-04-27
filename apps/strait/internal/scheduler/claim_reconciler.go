package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/store"
)

// ClaimReconciler periodically compares job_run_queue against job_runs
// to detect and fix drift caused by missed triggers, partial failures,
// or direct SQL modifications.
//
// Drift types detected:
//   - Missing claims: job_runs row is queued/delayed but has no claim row.
//   - Stale claims: claim row exists but job_runs row is no longer queued/delayed.
type ClaimReconciler struct {
	db       store.DBTX
	interval time.Duration
	logger   *slog.Logger
}

// NewClaimReconciler creates a reconciler. Zero interval defaults to 5m.
func NewClaimReconciler(db store.DBTX, interval time.Duration) *ClaimReconciler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &ClaimReconciler{
		db:       db,
		interval: interval,
		logger:   slog.Default(),
	}
}

// Run starts the periodic reconciliation loop.
func (r *ClaimReconciler) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("claim-reconciler", r.interval, r.logger, func(loopCtx context.Context) {
		if err := r.reconcileOnce(loopCtx); err != nil {
			r.logger.Error("claim reconciler failed", "error", err)
		}
	})
	loop.Run(ctx)
}

func (r *ClaimReconciler) reconcileOnce(ctx context.Context) error {
	// 1. Find missing claim rows: queued/delayed in job_runs but not in job_run_queue.
	missingSQL := `
		INSERT INTO job_run_queue (
			run_id, job_id, project_id, priority, created_at,
			scheduled_at, next_retry_at, concurrency_key,
			job_max_concurrency, job_max_concurrency_per_key,
			job_enabled, job_paused
		)
		SELECT
			jr.id, jr.job_id, jr.project_id, jr.priority, jr.created_at,
			jr.scheduled_at, jr.next_retry_at, jr.concurrency_key,
			j.max_concurrency, j.max_concurrency_per_key,
			j.enabled, j.paused
		FROM job_runs jr
		JOIN jobs j ON j.id = jr.job_id
		LEFT JOIN job_run_queue q ON q.run_id = jr.id
		WHERE jr.status IN ('queued', 'delayed')
		  AND q.run_id IS NULL
		LIMIT 1000
		ON CONFLICT (run_id) DO NOTHING`

	tag, err := r.db.Exec(ctx, missingSQL)
	if err != nil {
		return fmt.Errorf("reconcile missing claims: %w", err)
	}
	if inserted := tag.RowsAffected(); inserted > 0 {
		r.logger.Warn("claim reconciler: repaired missing claim rows", "count", inserted)
	}

	// 2. Find stale claim rows: in job_run_queue but job_runs is not queued/delayed.
	staleSQL := `
		DELETE FROM job_run_queue
		WHERE run_id IN (
			SELECT q.run_id
			FROM job_run_queue q
			LEFT JOIN job_runs jr ON jr.id = q.run_id
			WHERE jr.id IS NULL
			   OR jr.status NOT IN ('queued', 'delayed')
			LIMIT 1000
		)`

	tag, err = r.db.Exec(ctx, staleSQL)
	if err != nil {
		return fmt.Errorf("reconcile stale claims: %w", err)
	}
	if deleted := tag.RowsAffected(); deleted > 0 {
		r.logger.Warn("claim reconciler: removed stale claim rows", "count", deleted)
	}

	return nil
}
