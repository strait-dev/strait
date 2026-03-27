package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func workflowVersionSnapshotID(workflowID string, version int) string {
	return fmt.Sprintf("%s:v%d", workflowID, version)
}

func (q *Queries) CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowVersionSnapshot")
	defer span.End()

	versionID := workflowVersionSnapshotID(workflowID, version)

	insertVersion := `
		INSERT INTO workflow_versions (
			id, workflow_id, version, project_id, name, slug, description, enabled,
			timeout_secs, max_concurrent_runs, max_parallel_steps, cron, cron_timezone, skip_if_running,
			version_id, created_by, updated_by
		)
		SELECT $1, id, version, project_id, name, slug, description, enabled,
		       timeout_secs, max_concurrent_runs, max_parallel_steps, cron, cron_timezone, skip_if_running,
		       COALESCE(version_id, ''), COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM workflows
		WHERE id = $2 AND version = $3
		ON CONFLICT (workflow_id, version)
		DO UPDATE SET
			project_id = EXCLUDED.project_id,
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			timeout_secs = EXCLUDED.timeout_secs,
			max_concurrent_runs = EXCLUDED.max_concurrent_runs,
			max_parallel_steps = EXCLUDED.max_parallel_steps,
			cron = EXCLUDED.cron,
			cron_timezone = EXCLUDED.cron_timezone,
			skip_if_running = EXCLUDED.skip_if_running,
			version_id = EXCLUDED.version_id,
			created_by = EXCLUDED.created_by,
			updated_by = EXCLUDED.updated_by`

	tag, err := q.db.Exec(ctx, insertVersion, versionID, workflowID, version)
	if err != nil {
		return fmt.Errorf("insert workflow version snapshot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWorkflowNotFound
	}

	if _, err := q.db.Exec(ctx, `DELETE FROM workflow_version_steps WHERE workflow_version_id = $1`, versionID); err != nil {
		return fmt.Errorf("clear workflow version steps: %w", err)
	}

	insertSteps := `
		INSERT INTO workflow_version_steps (
			id, workflow_version_id, job_id, step_ref, depends_on, condition, on_failure, payload,
			step_type, approval_timeout_secs, approval_approvers,
			retry_max_attempts, retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
			timeout_secs_override, output_transform,
			sub_workflow_id, max_nesting_depth,
			event_key, event_timeout_secs, event_notify_url, sleep_duration_secs, event_emit_key,
			concurrency_key, resource_class,
			cost_gate_threshold_microusd, cost_gate_timeout_secs, cost_gate_default_action,
			expected_duration_secs, stage_notifications,
			compensation_job_id, compensation_timeout_secs
		)
		SELECT
			$1 || ':' || step_ref,
			$1,
			job_id,
			step_ref,
			depends_on,
			condition,
			on_failure,
			payload,
			step_type,
			approval_timeout_secs,
			approval_approvers,
			retry_max_attempts,
			retry_backoff,
			retry_initial_delay_secs,
			retry_max_delay_secs,
			timeout_secs_override,
			output_transform,
			sub_workflow_id,
			max_nesting_depth,
			event_key,
			event_timeout_secs,
			event_notify_url,
			sleep_duration_secs,
			event_emit_key,
			concurrency_key,
			resource_class,
			cost_gate_threshold_microusd,
			cost_gate_timeout_secs,
			cost_gate_default_action,
			expected_duration_secs,
			stage_notifications,
			compensation_job_id,
			compensation_timeout_secs
		FROM workflow_steps
		WHERE workflow_id = $2`

	if _, err := q.db.Exec(ctx, insertSteps, versionID, workflowID); err != nil {
		return fmt.Errorf("insert workflow version steps snapshot: %w", err)
	}

	return nil
}

func (q *Queries) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStepsByWorkflowVersion")
	defer span.End()

	query := `
		SELECT
			ws.id,
			wv.workflow_id,
			wvs.job_id,
			wvs.step_ref,
			wvs.depends_on,
			wvs.condition,
			wvs.on_failure,
			wvs.payload,
			wvs.step_type,
			wvs.approval_timeout_secs,
			wvs.approval_approvers,
			wvs.retry_max_attempts,
			wvs.retry_backoff,
			wvs.retry_initial_delay_secs,
			wvs.retry_max_delay_secs,
			wvs.timeout_secs_override,
			wvs.output_transform,
			wvs.sub_workflow_id,
			wvs.max_nesting_depth,
			wvs.event_key,
			wvs.event_timeout_secs,
			wvs.event_notify_url,
			wvs.sleep_duration_secs,
			wvs.event_emit_key,
			wvs.concurrency_key,
			wvs.resource_class,
			wvs.cost_gate_threshold_microusd,
			wvs.cost_gate_timeout_secs,
			wvs.cost_gate_default_action,
			wvs.expected_duration_secs,
			wvs.stage_notifications,
			wvs.compensation_job_id,
			wvs.compensation_timeout_secs,
			wvs.created_at
		FROM workflow_version_steps wvs
		JOIN workflow_versions wv ON wv.id = wvs.workflow_version_id
		JOIN workflow_steps ws ON ws.workflow_id = wv.workflow_id AND ws.step_ref = wvs.step_ref
		WHERE wv.workflow_id = $1 AND wv.version = $2
		ORDER BY wvs.created_at ASC`

	rows, err := q.db.Query(ctx, query, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("list steps by workflow version: %w", err)
	}
	defer rows.Close()

	steps := make([]domain.WorkflowStep, 0, 16)
	for rows.Next() {
		step, scanErr := scanWorkflowStep(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list steps by workflow version scan: %w", scanErr)
		}
		steps = append(steps, *step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list steps by workflow version rows: %w", err)
	}

	return steps, nil
}

func (q *Queries) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunningWorkflowRuns")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM workflow_runs WHERE workflow_id = $1 AND status = 'running'`,
		workflowID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count running workflow runs: %w", err)
	}

	return count, nil
}

// ListWorkflowVersions returns version snapshots for a workflow, newest first.
func (q *Queries) ListWorkflowVersions(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowVersions")
	defer span.End()

	query := `
		SELECT id, workflow_id, version, project_id, name, slug,
		       COALESCE(description, ''), enabled,
		       timeout_secs, max_concurrent_runs, max_parallel_steps,
		       COALESCE(cron, ''), COALESCE(cron_timezone, ''), skip_if_running,
		       COALESCE(version_id, ''), COALESCE(created_by, ''), COALESCE(updated_by, ''),
		       created_at
		FROM workflow_versions
		WHERE workflow_id = $1
		ORDER BY version DESC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, workflowID, limit)
	if err != nil {
		return nil, fmt.Errorf("list workflow versions: %w", err)
	}
	defer rows.Close()

	versions := make([]domain.WorkflowVersion, 0, limit)
	for rows.Next() {
		var v domain.WorkflowVersion
		if err := rows.Scan(
			&v.ID, &v.WorkflowID, &v.Version, &v.ProjectID, &v.Name, &v.Slug,
			&v.Description, &v.Enabled, &v.TimeoutSecs, &v.MaxConcurrentRuns,
			&v.MaxParallelSteps, &v.Cron, &v.CronTimezone, &v.SkipIfRunning,
			&v.VersionID, &v.CreatedBy, &v.UpdatedBy, &v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workflow version: %w", err)
		}
		versions = append(versions, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow versions rows: %w", err)
	}
	return versions, nil
}

// GetWorkflowVersionByVersionID retrieves a single workflow version by its nanoid version_id.
func (q *Queries) GetWorkflowVersionByVersionID(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowVersionByVersionID")
	defer span.End()

	query := `
		SELECT id, workflow_id, version, project_id, name, slug,
		       COALESCE(description, ''), enabled,
		       timeout_secs, max_concurrent_runs, max_parallel_steps,
		       COALESCE(cron, ''), COALESCE(cron_timezone, ''), skip_if_running,
		       COALESCE(version_id, ''), COALESCE(created_by, ''), COALESCE(updated_by, ''),
		       created_at
		FROM workflow_versions
		WHERE workflow_id = $1 AND version_id = $2`

	var v domain.WorkflowVersion
	err := q.db.QueryRow(ctx, query, workflowID, versionID).Scan(
		&v.ID, &v.WorkflowID, &v.Version, &v.ProjectID, &v.Name, &v.Slug,
		&v.Description, &v.Enabled, &v.TimeoutSecs, &v.MaxConcurrentRuns,
		&v.MaxParallelSteps, &v.Cron, &v.CronTimezone, &v.SkipIfRunning,
		&v.VersionID, &v.CreatedBy, &v.UpdatedBy, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowVersionNotFound
		}
		return nil, fmt.Errorf("get workflow version by version_id: %w", err)
	}
	return &v, nil
}

func (q *Queries) ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListTimedOutWorkflowRuns")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs
		WHERE status IN ('running', 'paused')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT 1000`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list timed out workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 16)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list timed out workflow runs scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list timed out workflow runs rows: %w", err)
	}

	return runs, nil
}
