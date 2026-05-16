package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/queue"
	"strait/internal/store"
)

// Counter drift reconciler.
//
// The job_active_counts and dlq_counts tables are maintained by BEFORE/AFTER
// triggers on job_runs. Triggers can drift from ground truth if:
//   - A migration inadvertently disables them.
//   - A replica writes with session_replication_role = replica.
//   - An operator issues COPY or bulk INSERT bypassing triggers.
//   - A historical Postgres bug fires the trigger more than once per event.
//
// This reconciler runs periodically under a dedicated advisory lock, diffs
// each counter table against a GROUP BY COUNT of job_runs, and corrects any
// drift. The total absolute delta is exported as a gauge so a Grafana alert
// can fire when drift is persistent (implying the trigger is broken).

const reconcilerAdvisoryLockID int64 = 0x537452636E636C72 // "StRcnclr"

// CounterReconciler periodically re-syncs job_active_counts and dlq_counts.
type CounterReconciler struct {
	db             store.DBTX
	advisoryLocker AdvisoryLocker
	metrics        *queue.QueueMetrics
	interval       time.Duration
	logger         *slog.Logger
	iterations     atomic.Int64
	totalDrift     atomic.Int64
}

// CounterReconcilerConfig configures the reconciler.
type CounterReconcilerConfig struct {
	Interval time.Duration
	Logger   *slog.Logger
}

// NewCounterReconciler builds a reconciler. Zero interval defaults to 1h.
func NewCounterReconciler(db store.DBTX, cfg CounterReconcilerConfig) *CounterReconciler {
	r := &CounterReconciler{
		db:       db,
		interval: cfg.Interval,
		logger:   cfg.Logger,
	}
	if r.interval <= 0 {
		r.interval = time.Hour
	}
	if r.logger == nil {
		r.logger = slog.Default()
	}
	if m, err := queue.Metrics(); err == nil {
		r.metrics = m
	}
	return r
}

// WithAdvisoryLocker enables single-leader reconciliation.
func (r *CounterReconciler) WithAdvisoryLocker(locker AdvisoryLocker) *CounterReconciler {
	r.advisoryLocker = locker
	return r
}

// Iterations returns completed reconciliation cycles. For tests.
func (r *CounterReconciler) Iterations() int64 { return r.iterations.Load() }

// TotalDrift returns the cumulative absolute drift observed across all
// runs. For tests and assertions.
func (r *CounterReconciler) TotalDrift() int64 { return r.totalDrift.Load() }

// Run blocks until ctx is cancelled; first tick runs immediately so tests
// don't have to wait a full interval.
func (r *CounterReconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, r.interval, func() {
		_ = r.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, r.interval, func() {
				_ = r.runOnce(ctx)
			})
		}
	}
}

// RunOnceForTest is an exported shim around runOnce for integration tests
// that cannot reach into the unexported method.
func (r *CounterReconciler) RunOnceForTest(ctx context.Context) error {
	return r.runOnce(ctx)
}

// runOnce executes a single reconciliation cycle. Exposed for tests.
func (r *CounterReconciler) runOnce(ctx context.Context) error {
	defer func() {
		r.iterations.Add(1)
		if rec := recover(); rec != nil {
			r.logger.Warn("counter reconciler panic recovered", "panic", rec)
		}
	}()

	acquired, err := runWithOptionalAdvisoryLock(ctx, r.advisoryLocker, reconcilerAdvisoryLockID, r.reconcileLocked)
	if err != nil {
		r.logger.Debug("reconciler lock cycle failed", "error", err)
		return err
	}
	if !acquired {
		return nil
	}
	return nil
}

func (r *CounterReconciler) reconcileLocked(ctx context.Context) error {
	if beginner, ok := r.db.(store.TxBeginner); ok {
		tx, err := beginner.Begin(ctx)
		if err != nil {
			return fmt.Errorf("counter reconciler begin tx: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()
		for _, stmt := range []string{
			`LOCK TABLE job_runs IN SHARE ROW EXCLUSIVE MODE`,
			`LOCK TABLE job_active_counts IN SHARE ROW EXCLUSIVE MODE`,
			`LOCK TABLE dlq_counts IN SHARE ROW EXCLUSIVE MODE`,
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("counter reconciler lock tables: %w", err)
			}
		}
		txReconciler := &CounterReconciler{
			db:      tx,
			metrics: r.metrics,
			logger:  r.logger,
		}
		txDrift := txReconciler.reconcileLockedNoTx(ctx)
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("counter reconciler commit: %w", err)
		}
		committed = true
		r.totalDrift.Add(txDrift)
		return nil
	}
	r.reconcileLockedNoTx(ctx)
	return nil
}

func (r *CounterReconciler) reconcileLockedNoTx(ctx context.Context) int64 {
	activeDrift, err := r.reconcileActiveCounts(ctx)
	if err != nil {
		r.logger.Warn("active counts reconcile failed", "error", err)
	}
	dlqDrift, err := r.reconcileDLQCounts(ctx)
	if err != nil {
		r.logger.Warn("dlq counts reconcile failed", "error", err)
	}

	drift := activeDrift + dlqDrift
	r.totalDrift.Add(drift)
	if r.metrics != nil {
		r.metrics.CounterDrift.Record(ctx, drift)
	}
	if drift > 0 {
		r.logger.Info("counter drift detected and corrected",
			"active_drift", activeDrift,
			"dlq_drift", dlqDrift,
		)
	}
	return drift
}

// reconcileActiveCounts replaces the job_active_counts table with the
// ground-truth aggregate from job_runs. Returns absolute drift (sum of |
// old - new | across rows).
func (r *CounterReconciler) reconcileActiveCounts(ctx context.Context) (int64, error) {
	const q = `
WITH truth AS (
    SELECT job_id, COALESCE(concurrency_key, '') AS concurrency_key, COUNT(*)::int AS count
    FROM job_runs
    WHERE status IN ('dequeued', 'executing')
    GROUP BY job_id, COALESCE(concurrency_key, '')
),
current AS (
    SELECT job_id, concurrency_key, count FROM job_active_counts
),
corrections AS (
    SELECT COALESCE(t.job_id, c.job_id)                     AS job_id,
           COALESCE(t.concurrency_key, c.concurrency_key)   AS concurrency_key,
           COALESCE(t.count, 0)                             AS truth_count,
           COALESCE(c.count, 0)                             AS current_count
    FROM truth t
    FULL OUTER JOIN current c
      ON c.job_id = t.job_id AND c.concurrency_key = t.concurrency_key
    WHERE COALESCE(t.count, 0) <> COALESCE(c.count, 0)
),
drift_total AS (
    SELECT COALESCE(SUM(ABS(truth_count - current_count)), 0)::bigint AS delta FROM corrections
),
apply AS (
    INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
    SELECT job_id, concurrency_key, truth_count, NOW() FROM corrections
    ON CONFLICT (job_id, concurrency_key)
    DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()
    RETURNING 1
),
-- Safety net: zero counter rows that have no matching truth rows.
-- Technically redundant with the apply CTE (FULL OUTER JOIN already
-- produces truth_count=0 for these rows), but kept as defense-in-depth
-- against CTE execution-order edge cases.
zeroed AS (
    UPDATE job_active_counts ac
    SET count = 0, updated_at = NOW()
    WHERE NOT EXISTS (
        SELECT 1 FROM truth t
        WHERE t.job_id = ac.job_id AND t.concurrency_key = ac.concurrency_key
    ) AND ac.count <> 0
    RETURNING 1
)
SELECT delta FROM drift_total
`
	var delta int64
	if err := r.db.QueryRow(ctx, q).Scan(&delta); err != nil {
		return 0, fmt.Errorf("reconcile active counts: %w", err)
	}
	return delta, nil
}

func (r *CounterReconciler) reconcileDLQCounts(ctx context.Context) (int64, error) {
	const q = `
WITH truth AS (
    SELECT project_id, job_id, COUNT(*)::int AS count
    FROM job_runs
    WHERE status = 'dead_letter'
      AND (visible_until IS NULL OR visible_until > NOW())
    GROUP BY project_id, job_id
),
current AS (
    SELECT project_id, job_id, count FROM dlq_counts
),
corrections AS (
    SELECT COALESCE(t.project_id, c.project_id) AS project_id,
           COALESCE(t.job_id, c.job_id)         AS job_id,
           COALESCE(t.count, 0)                 AS truth_count,
           COALESCE(c.count, 0)                 AS current_count
    FROM truth t
    FULL OUTER JOIN current c
      ON c.project_id = t.project_id AND c.job_id = t.job_id
    WHERE COALESCE(t.count, 0) <> COALESCE(c.count, 0)
),
drift_total AS (
    SELECT COALESCE(SUM(ABS(truth_count - current_count)), 0)::bigint AS delta FROM corrections
),
apply AS (
    INSERT INTO dlq_counts (project_id, job_id, count, updated_at)
    SELECT project_id, job_id, truth_count, NOW() FROM corrections
    ON CONFLICT (project_id, job_id)
    DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()
    RETURNING 1
),
-- Safety net: zero counter rows that have no matching truth rows.
-- Technically redundant with the apply CTE (FULL OUTER JOIN already
-- produces truth_count=0 for these rows), but kept as defense-in-depth
-- against CTE execution-order edge cases.
zeroed AS (
    UPDATE dlq_counts dc
    SET count = 0, updated_at = NOW()
    WHERE NOT EXISTS (
        SELECT 1 FROM truth t
        WHERE t.project_id = dc.project_id AND t.job_id = dc.job_id
    ) AND dc.count <> 0
    RETURNING 1
)
SELECT delta FROM drift_total
`
	var delta int64
	if err := r.db.QueryRow(ctx, q).Scan(&delta); err != nil {
		return 0, fmt.Errorf("reconcile dlq counts: %w", err)
	}
	return delta, nil
}
