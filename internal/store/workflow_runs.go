package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateWorkflowRun")
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

	query := `
		INSERT INTO workflow_runs (
			id, workflow_id, project_id, status, triggered_by, payload,
			workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			retry_of_run_id, parent_workflow_run_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow run: %w", err)
	}

	return nil
}

func (q *Queries) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWorkflowRun")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, created_at
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

func (q *Queries) ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListWorkflowRuns")
	defer span.End()

	var rows pgx.Rows
	var err error

	if cursor != nil {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, created_at
			FROM workflow_runs
			WHERE workflow_id = $1 AND created_at < $3
			ORDER BY created_at DESC
			LIMIT $2`
		rows, err = q.db.Query(ctx, query, workflowID, limit, *cursor)
	} else {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, created_at
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListWorkflowRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, created_at
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteWorkflowRunsFinishedBefore")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH doomed AS (
			SELECT id
			FROM workflow_runs
			WHERE status IN ('completed', 'failed', 'canceled')
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateWorkflowRunStatus")
	defer span.End()

	if err := domain.ValidateWorkflowTransition(from, to); err != nil {
		return fmt.Errorf("invalid workflow status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"triggered_by": {},
		"payload":      {},
		"error":        {},
		"started_at":   {},
		"finished_at":  {},
		"expires_at":   {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := lo.Keys(fields)
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
		return fmt.Errorf("update workflow run status conflict: id %s from %s", id, from)
	}

	return nil
}

// GetWorkflowRunsByParent returns all child workflow runs for a given parent workflow run.
func (q *Queries) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWorkflowRunsByParent")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, created_at
		FROM workflow_runs
		WHERE parent_workflow_run_id = $1
		ORDER BY created_at ASC`

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
	var run domain.WorkflowRun
	var payload []byte
	var runError *string
	var startedAt *time.Time
	var finishedAt *time.Time
	var expiresAt *time.Time
	var retryOfRunID *string
	var parentWorkflowRunID *string

	err := scanner.Scan(
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
		&run.CreatedAt,
	)
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
	run.StartedAt = startedAt
	run.FinishedAt = finishedAt
	run.ExpiresAt = expiresAt

	return &run, nil
}
