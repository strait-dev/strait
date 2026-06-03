package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// StreamJobs iterates all jobs for a project, calling fn per row.
// Rows are ordered by created_at ASC for deterministic export order.
//
// The SELECT list must match the column order expected by scanJob. If
// new columns are added to the jobs table they must be added here too,
// otherwise pgx returns "number of field descriptions must equal number
// of destinations" at scan time.
func (q *Queries) StreamJobs(ctx context.Context, projectID string, fn func(*domain.Job) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamJobs")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata,
		       retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold,
			       cron_overlap_policy, result_schema,
			       debounce_window_secs, batch_window_secs, batch_max_size,
			       execution_mode, preferred_regions, queue_name,
			       on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping,
			       on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
			FROM jobs
		WHERE project_id = $1
		ORDER BY created_at ASC
		LIMIT 1000000`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("stream jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return fmt.Errorf("stream jobs scan: %w", scanErr)
		}
		if err := fn(job); err != nil {
			return err
		}
	}
	return rows.Err()
}

// StreamWorkflows iterates all workflows for a project, calling fn per row.
func (q *Queries) StreamWorkflows(ctx context.Context, projectID string, fn func(*domain.Workflow) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamWorkflows")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at
		FROM workflows
		WHERE project_id = $1
		ORDER BY created_at ASC
		LIMIT 1000000`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("stream workflows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		w, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return fmt.Errorf("stream workflows scan: %w", scanErr)
		}
		if err := fn(w); err != nil {
			return err
		}
	}
	return rows.Err()
}

// StreamRuns iterates runs for a project within a time window, calling fn per row.
func (q *Queries) StreamRuns(ctx context.Context, projectID string, from, to time.Time, fn func(*domain.JobRun) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamRuns")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at,
		       workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags,
		       job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE project_id = $1
		  AND created_at >= $2
		  AND created_at <= $3
		ORDER BY created_at ASC
		LIMIT 1000000`

	rows, err := q.db.Query(ctx, query, projectID, from, to)
	if err != nil {
		return fmt.Errorf("stream runs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return fmt.Errorf("stream runs scan: %w", scanErr)
		}
		if err := fn(run); err != nil {
			return err
		}
	}
	return rows.Err()
}
