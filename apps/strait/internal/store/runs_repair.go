package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CountStrandedTerminalRuns(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountStrandedTerminalRuns")
	defer span.End()

	shortCutoff := time.Now().Add(-shortRetention)
	longCutoff := time.Now().Add(-longRetention)

	query := `
		SELECT COUNT(*) FROM job_runs
		WHERE finished_at IS NOT NULL
		  AND (
			(status IN ('completed', 'failed', 'canceled', 'expired') AND finished_at <= $1)
			OR
			(status IN ('timed_out', 'crashed', 'system_failed') AND finished_at <= $2)
		  )`

	var count int64
	if err := q.db.QueryRow(ctx, query, shortCutoff, longCutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stranded terminal runs: %w", err)
	}
	return count, nil
}

func (q *Queries) CountDuplicateHistoryRuns(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountDuplicateHistoryRuns")
	defer span.End()

	// Count all duplicates, not a bounded sample: the previous LIMIT 10000 inside
	// the subquery silently saturated the result at 10 000, so an operator could
	// not tell 10 000 orphans from hundreds of thousands and would under-size the
	// subsequent repair.
	query := `
		SELECT COUNT(*) FROM job_runs jr
		INNER JOIN job_runs_history jrh ON jr.id = jrh.id`

	var count int64
	if err := q.db.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count duplicate history runs: %w", err)
	}
	return count, nil
}

func (q *Queries) RepairOrphanedHistoryRuns(ctx context.Context, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RepairOrphanedHistoryRuns")
	defer span.End()

	query := `
		WITH victims AS MATERIALIZED (
			SELECT jr.id FROM job_runs jr
			INNER JOIN job_runs_history jrh ON jr.id = jrh.id
			WHERE jr.finished_at IS NOT NULL
			  AND jr.status IN ('completed', 'failed', 'canceled', 'expired', 'timed_out', 'crashed', 'system_failed')
			LIMIT $1
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM victims)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM victims)
		)
		DELETE FROM job_runs
		WHERE id IN (SELECT id FROM victims)`

	tag, err := q.db.Exec(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("repair orphaned history runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) BackfillTerminalRunsToHistory(ctx context.Context, finishedBefore time.Time, batchSize int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BackfillTerminalRunsToHistory")
	defer span.End()

	query := `
		WITH to_archive AS (
			SELECT id FROM job_runs
			WHERE finished_at IS NOT NULL
			  AND finished_at <= $1
			  AND status IN ('completed', 'failed', 'canceled', 'expired', 'timed_out', 'crashed', 'system_failed')
			ORDER BY finished_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		),
		archived AS (
			INSERT INTO job_runs_history (` + historyArchiveColumns + `)
			SELECT ` + historyArchiveColumns + `
			FROM job_runs
			WHERE id IN (SELECT id FROM to_archive)
			ON CONFLICT (id) DO NOTHING
			RETURNING id
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM archived)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM archived)
		)
		DELETE FROM job_runs WHERE id IN (SELECT id FROM archived)`

	tag, err := q.db.Exec(ctx, query, finishedBefore, batchSize)
	if err != nil {
		return 0, fmt.Errorf("backfill terminal runs to history: %w", err)
	}
	return tag.RowsAffected(), nil
}
