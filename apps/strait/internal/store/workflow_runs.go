package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}
	if run.Status == "" {
		run.Status = domain.WfStatusPending
	}
	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}
	if run.WorkflowVersion == 0 {
		run.WorkflowVersion = 1
	}
	if run.WorkflowSnapshotID != "" {
		var snapshotExists bool
		if err := q.db.QueryRow(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM workflow_snapshots WHERE id = $1)`,
			run.WorkflowSnapshotID,
		).Scan(&snapshotExists); err != nil {
			return fmt.Errorf("create workflow run: verify workflow snapshot %q: %w", run.WorkflowSnapshotID, err)
		}
		if !snapshotExists {
			return fmt.Errorf("create workflow run: workflow snapshot %q not found", run.WorkflowSnapshotID)
		}
	}

	tagsJSON := []byte("{}")
	if len(run.Tags) > 0 {
		var marshalErr error
		tagsJSON, marshalErr = json.Marshal(run.Tags)
		if marshalErr != nil {
			return fmt.Errorf("create workflow run: marshal tags: %w", marshalErr)
		}
	}

	var traceContextJSON []byte
	if len(run.TraceContext) > 0 {
		var marshalErr error
		traceContextJSON, marshalErr = json.Marshal(run.TraceContext)
		if marshalErr != nil {
			return fmt.Errorf("create workflow run: marshal trace_context: %w", marshalErr)
		}
	}

	query := `
		INSERT INTO workflow_runs (
			id, workflow_id, project_id, status, triggered_by, payload,
			workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			retry_of_run_id, parent_workflow_run_id, parent_step_run_id,
			tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id,
			expected_completion_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16::jsonb, $17, $18, $19::jsonb, $20, $21)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		run.ID,
		run.WorkflowID,
		run.ProjectID,
		run.Status,
		run.TriggeredBy,
		dbscan.NilIfEmptyRawMessage(run.Payload),
		run.WorkflowVersion,
		run.MaxParallelSteps,
		dbscan.NilIfEmptyString(run.Error),
		run.StartedAt,
		run.FinishedAt,
		run.ExpiresAt,
		dbscan.NilIfEmptyString(run.RetryOfRunID),
		dbscan.NilIfEmptyString(run.ParentWorkflowRunID),
		dbscan.NilIfEmptyString(run.ParentStepRunID),
		tagsJSON,
		dbscan.NilIfEmptyString(run.WorkflowVersionID),
		dbscan.NilIfEmptyString(run.CreatedBy),
		traceContextJSON,
		dbscan.NilIfEmptyString(run.WorkflowSnapshotID),
		run.ExpectedCompletionAt,
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow run snapshot_id=%q: %w", run.WorkflowSnapshotID, err)
	}

	return nil
}

func (q *Queries) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRun")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs
		WHERE id = $1`

	run, err := scanWorkflowRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowRunNotFound
		}
		return nil, fmt.Errorf("get workflow run: %w", err)
	}

	return run, nil
}

func (q *Queries) GetWorkflowRunWithCacheVersion(ctx context.Context, id string) (*domain.WorkflowRun, int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRunWithCacheVersion")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at, cache_version
		FROM workflow_runs
		WHERE id = $1`

	run, err := scanWorkflowRunWithCacheVersion(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrWorkflowRunNotFound
		}
		return nil, 0, fmt.Errorf("get workflow run with cache version: %w", err)
	}

	return run, run.CacheVersion, nil
}

func (q *Queries) ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRuns")
	defer span.End()

	var rows pgx.Rows
	var err error

	if cursor != nil {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
			FROM workflow_runs
			WHERE workflow_id = $1 AND created_at < $3
			ORDER BY created_at DESC
			LIMIT $2`
		rows, err = q.db.Query(ctx, query, workflowID, limit, *cursor)
	} else {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
			FROM workflow_runs
			WHERE workflow_id = $1
			ORDER BY created_at DESC
			LIMIT $2`
		rows, err = q.db.Query(ctx, query, workflowID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs
		WHERE project_id = $1`

	args := []any{projectID}
	param := 2

	if status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, *status)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 16)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs by project scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs by project rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteWorkflowRunsFinishedBefore")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH doomed AS (
			SELECT id
			FROM workflow_runs
			WHERE status IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed')
			  AND finished_at IS NOT NULL
			  AND finished_at < $1
			ORDER BY finished_at ASC
			LIMIT $2
		)
		DELETE FROM workflow_runs wr
		USING doomed
		WHERE wr.id = doomed.id`

	tag, err := q.db.Exec(ctx, query, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete old workflow runs: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (q *Queries) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateWorkflowRunStatus")
	defer span.End()

	if err := domain.ValidateWorkflowTransition(from, to); err != nil {
		return fmt.Errorf("invalid workflow status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"triggered_by":           {},
		"payload":                {},
		"error":                  {},
		"started_at":             {},
		"finished_at":            {},
		"expires_at":             {},
		"expected_completion_at": {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}

		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "error" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE workflow_runs SET %s WHERE id = $2 AND status = $3",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update workflow run status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		var currentStatus domain.WorkflowRunStatus
		err := q.db.QueryRow(ctx, "SELECT status FROM workflow_runs WHERE id = $1", id).Scan(&currentStatus)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrWorkflowRunNotFound
			}
			return fmt.Errorf("checking current workflow status: %w", err)
		}
		if currentStatus == to {
			return nil // idempotent: already in target state
		}
		return fmt.Errorf("update workflow run status conflict: id %s from %s", id, from)
	}

	return nil
}

// GetWorkflowRunsByParent returns all child workflow runs for a given parent workflow run.
func (q *Queries) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRunsByParent")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs
		WHERE parent_workflow_run_id = $1
		ORDER BY created_at ASC
		LIMIT 10000`

	rows, err := q.db.Query(ctx, query, parentWorkflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow runs by parent: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 8)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("get workflow runs by parent scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get workflow runs by parent rows: %w", err)
	}

	return runs, nil
}

func scanWorkflowRun(scanner scanTarget) (*domain.WorkflowRun, error) {
	return scanWorkflowRunFields(scanner, false)
}

func scanWorkflowRunWithCacheVersion(scanner scanTarget) (*domain.WorkflowRun, error) {
	return scanWorkflowRunFields(scanner, true)
}

func scanWorkflowRunFields(scanner scanTarget, includeCacheVersion bool) (*domain.WorkflowRun, error) {
	var run domain.WorkflowRun
	var payload []byte
	var tagsJSON []byte
	var runError *string
	var startedAt *time.Time
	var finishedAt *time.Time
	var expiresAt *time.Time
	var retryOfRunID *string
	var parentWorkflowRunID *string
	var parentStepRunID *string
	var workflowVersionID *string
	var createdBy *string
	var traceContextJSON []byte
	var workflowSnapshotID *string
	var expectedCompletionAt *time.Time
	dest := []any{
		&run.ID,
		&run.WorkflowID,
		&run.ProjectID,
		&run.Status,
		&run.TriggeredBy,
		&payload,
		&run.WorkflowVersion,
		&run.MaxParallelSteps,
		&runError,
		&startedAt,
		&finishedAt,
		&expiresAt,
		&retryOfRunID,
		&parentWorkflowRunID,
		&parentStepRunID,
		&run.CreatedAt,
		&tagsJSON,
		&workflowVersionID,
		&createdBy,
		&traceContextJSON,
		&workflowSnapshotID,
		&expectedCompletionAt,
	}
	if includeCacheVersion {
		dest = append(dest, &run.CacheVersion)
	}

	err := scanner.Scan(dest...)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		run.Payload = json.RawMessage(payload)
	}
	if runError != nil {
		run.Error = *runError
	}
	if retryOfRunID != nil {
		run.RetryOfRunID = *retryOfRunID
	}
	if parentWorkflowRunID != nil {
		run.ParentWorkflowRunID = *parentWorkflowRunID
	}
	if parentStepRunID != nil {
		run.ParentStepRunID = *parentStepRunID
	}
	run.StartedAt = startedAt
	run.FinishedAt = finishedAt
	run.ExpiresAt = expiresAt
	if len(tagsJSON) > 0 && string(tagsJSON) != "{}" {
		if err := json.Unmarshal(tagsJSON, &run.Tags); err != nil {
			return nil, err
		}
	}
	if workflowVersionID != nil {
		run.WorkflowVersionID = *workflowVersionID
	}
	if createdBy != nil {
		run.CreatedBy = *createdBy
	}
	if len(traceContextJSON) > 0 && string(traceContextJSON) != "{}" {
		if err := json.Unmarshal(traceContextJSON, &run.TraceContext); err != nil {
			return nil, err
		}
	}
	if workflowSnapshotID != nil {
		run.WorkflowSnapshotID = *workflowSnapshotID
	}
	run.ExpectedCompletionAt = expectedCompletionAt

	return &run, nil
}

func (q *Queries) CreateWorkflowRunBootstrap(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRunBootstrap")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			return err
		}
		if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
			return err
		}
		for i := range stepRuns {
			sr := stepRuns[i]
			if err := q.CreateWorkflowStepRun(ctx, &sr); err != nil {
				return err
			}
		}
		return nil
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		if err := txQ.CreateWorkflowRun(ctx, run); err != nil {
			return fmt.Errorf("create workflow run bootstrap: %w", err)
		}
		if err := txQ.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
			return fmt.Errorf("mark workflow running bootstrap: %w", err)
		}
		for i := range stepRuns {
			sr := stepRuns[i]
			if err := txQ.CreateWorkflowStepRun(ctx, &sr); err != nil {
				return fmt.Errorf("create workflow step run bootstrap %s: %w", sr.StepRef, err)
			}
		}
		return nil
	})
}

func (q *Queries) ListStalledWorkflowRuns(ctx context.Context, threshold time.Duration) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStalledWorkflowRuns")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs wr
		WHERE wr.status = 'running'
		  AND wr.started_at IS NOT NULL
		  AND wr.started_at < NOW() - ($1::interval)
		  AND NOT EXISTS (
			SELECT 1 FROM workflow_step_runs sr
			WHERE sr.workflow_run_id = wr.id
			  AND sr.status = 'running'
		  )
		ORDER BY wr.started_at ASC
		LIMIT 200`

	interval := fmt.Sprintf("%f seconds", threshold.Seconds())
	rows, err := q.db.Query(ctx, query, interval)
	if err != nil {
		return nil, fmt.Errorf("list stalled workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 16)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list stalled workflow runs scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stalled workflow runs rows: %w", err)
	}
	return runs, nil
}

func (q *Queries) ListWorkflowRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRunsByTag")
	defer span.End()

	base := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at
		FROM workflow_runs
		WHERE project_id = $1`

	args := []any{projectID, tagKey}
	param := 3
	if tagValue == "" {
		base += ` AND tags ? $2`
	} else {
		base += ` AND tags ->> $2 = $3`
		args = append(args, tagValue)
		param++
	}

	if cursor != nil {
		base += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs by tag: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs by tag scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs by tag rows: %w", err)
	}

	return runs, nil
}

// CountActiveWorkflowRunsByVersion returns the number of non-terminal workflow runs for a specific version.
func (q *Queries) CountActiveWorkflowRunsByVersion(ctx context.Context, workflowID, versionID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountActiveWorkflowRunsByVersion")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_runs
		WHERE workflow_id = $1
		  AND workflow_version_id = $2
		  AND status IN ('pending', 'running', 'paused')
	`, workflowID, versionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active workflow runs by version: %w", err)
	}
	return count, nil
}

// ActiveVersion represents a workflow version with active run counts.
type ActiveVersion struct {
	VersionID string `json:"version_id"`
	Version   int    `json:"version"`
	Pending   int    `json:"pending"`
	Running   int    `json:"running"`
	Paused    int    `json:"paused"`
	Total     int    `json:"total"`
}

// ListActiveWorkflowVersions groups active workflow runs by version with status counts.
func (q *Queries) ListActiveWorkflowVersions(ctx context.Context, workflowID string) ([]ActiveVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListActiveWorkflowVersions")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT
			COALESCE(workflow_version_id, ''),
			workflow_version,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'running') AS running,
			COUNT(*) FILTER (WHERE status = 'paused') AS paused,
			COUNT(*) AS total
		FROM workflow_runs
		WHERE workflow_id = $1
		  AND status IN ('pending', 'running', 'paused')
		GROUP BY workflow_version_id, workflow_version
		ORDER BY workflow_version DESC
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list active workflow versions: %w", err)
	}
	defer rows.Close()

	var versions []ActiveVersion
	for rows.Next() {
		var v ActiveVersion
		if err := rows.Scan(&v.VersionID, &v.Version, &v.Pending, &v.Running, &v.Paused, &v.Total); err != nil {
			return nil, fmt.Errorf("list active workflow versions scan: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (q *Queries) BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelWorkflowRuns")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		UPDATE workflow_runs
		SET status = 'canceled', finished_at = $2, error = 'canceled by user (bulk)'
		WHERE id = ANY($1) AND project_id = $3
		  AND status NOT IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed')
		RETURNING id
	`, ids, now, projectID)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel workflow runs: %w", err)
	}
	defer rows.Close()

	var canceled []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bulk cancel workflow runs scan: %w", err)
		}
		canceled = append(canceled, id)
	}
	return canceled, rows.Err()
}
