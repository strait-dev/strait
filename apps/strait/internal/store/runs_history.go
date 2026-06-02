package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

const historyArchiveColumns = `id, job_id, project_id, status, attempt, payload, result, metadata,
	error, error_class, triggered_by, scheduled_at, started_at, finished_at,
	heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
	idempotency_key, job_version, workflow_step_run_id, execution_trace,
	debug_mode, continuation_of, lineage_depth, tags, job_version_id,
	created_by, concurrency_key, batch_id, execution_mode,
	is_rollback, replayed_run_id, max_attempts_override, timeout_secs_override,
	retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
	visible_until, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key,
	queue_name, cache_version, created_at`

func (q *Queries) ArchiveTerminalRun(ctx context.Context, tx DBTX, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ArchiveTerminalRun")
	defer span.End()

	query := `
		WITH removed AS (
			DELETE FROM job_runs WHERE id = $1 RETURNING *
		),
		archived AS (
			INSERT INTO job_runs_history (` + historyArchiveColumns + `)
			SELECT ` + historyArchiveColumns + ` FROM removed
			ON CONFLICT (id) DO NOTHING
			RETURNING id
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM removed)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM removed)
		)
		SELECT 1`

	_, err := tx.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("archive terminal run %s: %w", id, err)
	}
	return nil
}

func (q *Queries) GetRunFromHistory(ctx context.Context, id string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunFromHistory")
	defer span.End()

	query := `SELECT id, job_id, project_id, status, attempt, payload, result, metadata,
		error, error_class, triggered_by, scheduled_at, started_at, finished_at,
		heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
		idempotency_key, job_version, created_at, workflow_step_run_id,
		execution_trace, debug_mode, continuation_of, lineage_depth, tags,
		job_version_id, created_by, batch_id, concurrency_key, execution_mode,
		is_rollback, replayed_run_id
		FROM job_runs_history WHERE id = $1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run from history: %w", err)
	}
	return run, nil
}

func (q *Queries) GetRunFromHistoryWithCacheVersion(ctx context.Context, id string) (*domain.JobRun, int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunFromHistoryWithCacheVersion")
	defer span.End()

	query := `SELECT id, job_id, project_id, status, attempt, payload, result, metadata,
		error, error_class, triggered_by, scheduled_at, started_at, finished_at,
		heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
		idempotency_key, job_version, created_at, workflow_step_run_id,
		execution_trace, debug_mode, continuation_of, lineage_depth, tags,
		job_version_id, created_by, batch_id, concurrency_key, execution_mode,
		is_rollback, replayed_run_id, cache_version
		FROM job_runs_history WHERE id = $1`

	run, err := dbscan.ScanRunWithCacheVersion(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("get run from history with cache version: %w", err)
	}
	return run, run.CacheVersion, nil
}

func (q *Queries) DeleteHistoryRunsPastRetention(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteHistoryRunsPastRetention")
	defer span.End()

	query := `
		DELETE FROM job_runs_history
		WHERE id IN (
			SELECT id FROM job_runs_history
			WHERE archived_at < $1
			LIMIT $2
		)`

	tag, err := q.db.Exec(ctx, query, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("delete history runs past retention: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) ArchiveTerminalRunsPastRetention(
	ctx context.Context,
	shortRetention, longRetention time.Duration,
	batchSize int,
) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ArchiveTerminalRunsPastRetention")
	defer span.End()

	shortCutoff := time.Now().Add(-shortRetention)
	longCutoff := time.Now().Add(-longRetention)

	// Exclude the current month's partition to avoid creating dead tuples
	// in the hot partition that the dequeue hot path scans. The DELETE
	// + INSERT INTO history CTE is particularly expensive: it creates
	// dead tuples on both the source table and generates WAL for the
	// history insert. By skipping the hot partition, we let pg_partman
	// handle cleanup when the entire partition ages out.
	hotBoundary := beginningOfMonth(time.Now())

	query := `
		WITH to_archive AS (
			SELECT id, created_at FROM job_runs
			WHERE finished_at IS NOT NULL
			  AND created_at < $4
			  AND (
				(status IN ('completed', 'failed', 'canceled', 'expired') AND finished_at <= $1)
				OR
				(status IN ('timed_out', 'crashed', 'system_failed') AND finished_at <= $2)
			  )
			ORDER BY finished_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		),
		archived AS (
			INSERT INTO job_runs_history (` + historyArchiveColumns + `)
			SELECT ` + historyArchiveColumns + `
			FROM job_runs jr
			WHERE jr.id IN (SELECT id FROM to_archive)
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

	tag, err := q.db.Exec(ctx, query, shortCutoff, longCutoff, batchSize, hotBoundary)
	if err != nil {
		return 0, fmt.Errorf("archive terminal runs: %w", err)
	}
	return tag.RowsAffected(), nil
}
