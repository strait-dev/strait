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
		DELETE FROM job_runs
		WHERE id IN (
			SELECT jr.id FROM job_runs jr
			INNER JOIN job_runs_history jrh ON jr.id = jrh.id
			WHERE jr.finished_at IS NOT NULL
			LIMIT $1
		)`

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
			INSERT INTO job_runs_history (
				id, job_id, project_id, status, attempt, payload, result, metadata,
				error, error_class, triggered_by, scheduled_at, started_at, finished_at,
				heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
				idempotency_key, job_version, workflow_step_run_id, execution_trace,
				debug_mode, continuation_of, lineage_depth, tags, job_version_id,
				created_by, concurrency_key, batch_id, execution_mode, machine_id,
				deployment_id, pinned_image_uri, pinned_image_digest, is_rollback,
				replayed_run_id, max_attempts_override, timeout_secs_override,
				retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
				created_at
			)
			SELECT
				id, job_id, project_id, status, attempt, payload, result, metadata,
				error, error_class, triggered_by, scheduled_at, started_at, finished_at,
				heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
				idempotency_key, job_version, workflow_step_run_id, execution_trace,
				debug_mode, continuation_of, lineage_depth, tags, job_version_id,
				created_by, concurrency_key, batch_id, execution_mode, machine_id,
				deployment_id, pinned_image_uri, pinned_image_digest, is_rollback,
				replayed_run_id, max_attempts_override, timeout_secs_override,
				retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
				created_at
			FROM job_runs
			WHERE id IN (SELECT id FROM to_archive)
			ON CONFLICT (id) DO NOTHING
			RETURNING id
		)
		DELETE FROM job_runs WHERE id IN (SELECT id FROM archived)`

	tag, err := q.db.Exec(ctx, query, finishedBefore, batchSize)
	if err != nil {
		return 0, fmt.Errorf("backfill terminal runs to history: %w", err)
	}
	return tag.RowsAffected(), nil
}
