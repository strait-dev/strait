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
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowStepRun")
	defer span.End()

	if sr.ID == "" {
		sr.ID = uuid.Must(uuid.NewV7()).String()
	}
	if sr.Status == "" {
		sr.Status = domain.StepPending
	}
	if sr.Attempt == 0 {
		sr.Attempt = 1
	}

	query := `
		INSERT INTO workflow_step_runs (
			id, workflow_run_id, workflow_step_id, step_ref, job_run_id, status,
			deps_completed, deps_required, output, error, started_at, finished_at, attempt
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		sr.ID,
		sr.WorkflowRunID,
		sr.WorkflowStepID,
		sr.StepRef,
		dbscan.NilIfEmptyString(sr.JobRunID),
		sr.Status,
		sr.DepsCompleted,
		sr.DepsRequired,
		dbscan.NilIfEmptyRawMessage(sr.Output),
		dbscan.NilIfEmptyString(sr.Error),
		sr.StartedAt,
		sr.FinishedAt,
		sr.Attempt,
	).Scan(&sr.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow step run: %w", err)
	}

	return nil
}

func (q *Queries) GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowStepRun")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_id, step_ref, job_run_id, status,
		       deps_completed, deps_required, output, error, started_at, finished_at, attempt, created_at
		FROM workflow_step_runs
		WHERE id = $1`

	sr, err := scanWorkflowStepRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowStepRunNotFound
		}
		return nil, fmt.Errorf("get workflow step run: %w", err)
	}

	return sr, nil
}

func (q *Queries) GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetStepRunByJobRunID")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_id, step_ref, job_run_id, status,
		       deps_completed, deps_required, output, error, started_at, finished_at, attempt, created_at
		FROM workflow_step_runs
		WHERE job_run_id = $1`

	sr, err := scanWorkflowStepRun(q.db.QueryRow(ctx, query, jobRunID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get step run by job run id: %w", err)
	}

	return sr, nil
}

func (q *Queries) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStepRunsByWorkflowRun")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_id, step_ref, job_run_id, status,
		       deps_completed, deps_required, output, error, started_at, finished_at, attempt, created_at
		FROM workflow_step_runs
		WHERE workflow_run_id = $1`

	args := []any{workflowRunID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list step runs by workflow run: %w", err)
	}
	defer rows.Close()

	stepRuns := make([]domain.WorkflowStepRun, 0, 16)
	for rows.Next() {
		sr, scanErr := scanWorkflowStepRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list step runs by workflow run scan: %w", scanErr)
		}
		stepRuns = append(stepRuns, *sr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list step runs by workflow run rows: %w", err)
	}

	return stepRuns, nil
}

func (q *Queries) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateStepRunStatus")
	defer span.End()

	allowedColumns := map[string]struct{}{
		"job_run_id":  {},
		"output":      {},
		"error":       {},
		"started_at":  {},
		"finished_at": {},
		"attempt":     {},
	}

	setClauses := []string{"status = $1"}
	args := []any{status, id}
	param := 3

	keys := lo.Keys(fields)
	sort.Strings(keys)

	for _, key := range keys {
		value := fields[key]
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}

		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "job_run_id" || key == "error" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE workflow_step_runs SET %s WHERE id = $2",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update step run status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrWorkflowStepRunNotFound, id)
	}

	return nil
}

func (q *Queries) UpdateStepRunStatusFrom(ctx context.Context, id string, from, to domain.StepRunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateStepRunStatusFrom")
	defer span.End()

	allowedColumns := map[string]struct{}{
		"job_run_id":  {},
		"output":      {},
		"error":       {},
		"started_at":  {},
		"finished_at": {},
		"attempt":     {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := lo.Keys(fields)
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "job_run_id" || key == "error" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf("UPDATE workflow_step_runs SET %s WHERE id = $2 AND status = $3", strings.Join(setClauses, ", "))
	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update step run status from: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update step run status conflict: id %s from %s", id, from)
	}
	return nil
}

func (q *Queries) IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]StepDepResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.IncrementStepDeps")
	defer span.End()

	query := `
		UPDATE workflow_step_runs wsr
		SET deps_completed = deps_completed + 1
		FROM workflow_steps ws
		WHERE wsr.workflow_step_id = ws.id
		  AND wsr.workflow_run_id = $1
		  AND wsr.status = 'waiting'
		  AND $2 = ANY(ws.depends_on)
		RETURNING wsr.id, wsr.step_ref, wsr.deps_completed, wsr.deps_required, ws.job_id, ws.condition, ws.payload, wsr.workflow_run_id`

	rows, err := q.db.Query(ctx, query, workflowRunID, completedStepRef)
	if err != nil {
		return nil, fmt.Errorf("increment step deps: %w", err)
	}
	defer rows.Close()

	results := make([]StepDepResult, 0, 4)
	for rows.Next() {
		var r StepDepResult
		var condition []byte
		var payload []byte
		if scanErr := rows.Scan(
			&r.StepRunID,
			&r.StepRef,
			&r.DepsCompleted,
			&r.DepsRequired,
			&r.JobID,
			&condition,
			&payload,
			&r.WorkflowRunID,
		); scanErr != nil {
			return nil, fmt.Errorf("increment step deps scan: %w", scanErr)
		}
		if condition != nil {
			r.Condition = json.RawMessage(condition)
		}
		if payload != nil {
			r.Payload = json.RawMessage(payload)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("increment step deps rows: %w", err)
	}

	return results, nil
}

func (q *Queries) IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.IncrementStepRunAttempt")
	defer span.End()

	query := `
		UPDATE workflow_step_runs
		SET attempt = $1
		WHERE id = $2
		AND attempt = $3`

	tag, err := q.db.Exec(ctx, query, newAttempt, id, newAttempt-1)
	if err != nil {
		return fmt.Errorf("increment step run attempt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrWorkflowStepRunNotFound, id)
	}
	return nil
}

func (q *Queries) GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetStepOutputs")
	defer span.End()

	query := `
		SELECT step_ref, output
		FROM workflow_step_runs
		WHERE workflow_run_id = $1 AND step_ref = ANY($2)`

	rows, err := q.db.Query(ctx, query, workflowRunID, stepRefs)
	if err != nil {
		return nil, fmt.Errorf("get step outputs: %w", err)
	}
	defer rows.Close()

	outputs := make(map[string]json.RawMessage, len(stepRefs))
	for rows.Next() {
		var stepRef string
		var output []byte
		if scanErr := rows.Scan(&stepRef, &output); scanErr != nil {
			return nil, fmt.Errorf("get step outputs scan: %w", scanErr)
		}
		if output != nil {
			outputs[stepRef] = json.RawMessage(output)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get step outputs rows: %w", err)
	}

	return outputs, nil
}

func (q *Queries) CountNonTerminalStepRuns(ctx context.Context, workflowRunID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountNonTerminalStepRuns")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM workflow_step_runs
		WHERE workflow_run_id = $1
		  AND status NOT IN ('completed', 'failed', 'skipped', 'canceled')`

	var count int
	if err := q.db.QueryRow(ctx, query, workflowRunID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count non-terminal step runs: %w", err)
	}
	return count, nil
}

func (q *Queries) ListFailedStepRunRefs(ctx context.Context, workflowRunID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListFailedStepRunRefs")
	defer span.End()

	rows, err := q.db.Query(ctx, `SELECT step_ref FROM workflow_step_runs WHERE workflow_run_id = $1 AND status = 'failed'`, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("list failed step refs: %w", err)
	}
	defer rows.Close()

	refs := make([]string, 0, 8)
	for rows.Next() {
		var ref string
		if scanErr := rows.Scan(&ref); scanErr != nil {
			return nil, fmt.Errorf("list failed step refs scan: %w", scanErr)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list failed step refs rows: %w", err)
	}
	return refs, nil
}

func (q *Queries) CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelNonTerminalStepRuns")
	defer span.End()

	query := `
		UPDATE workflow_step_runs
		SET status = 'canceled',
		    finished_at = $2,
		    error = NULLIF($3, '')
		WHERE workflow_run_id = $1
		  AND status NOT IN ('completed', 'failed', 'skipped', 'canceled')`

	tag, err := q.db.Exec(ctx, query, workflowRunID, finishedAt, reason)
	if err != nil {
		return 0, fmt.Errorf("cancel non-terminal step runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) SkipStepRunsByRefs(ctx context.Context, workflowRunID string, refs []string, finishedAt time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SkipStepRunsByRefs")
	defer span.End()

	if len(refs) == 0 {
		return 0, nil
	}

	query := `
		UPDATE workflow_step_runs
		SET status = 'skipped', finished_at = $3
		WHERE workflow_run_id = $1
		  AND step_ref = ANY($2)
		  AND status NOT IN ('completed', 'failed', 'skipped', 'canceled')`
	tag, err := q.db.Exec(ctx, query, workflowRunID, refs, finishedAt)
	if err != nil {
		return 0, fmt.Errorf("skip step runs by refs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanWorkflowStepRun(scanner scanTarget) (*domain.WorkflowStepRun, error) {
	var sr domain.WorkflowStepRun
	var jobRunID *string
	var output []byte
	var stepRunError *string
	var startedAt *time.Time
	var finishedAt *time.Time
	var attempt int

	err := scanner.Scan(
		&sr.ID,
		&sr.WorkflowRunID,
		&sr.WorkflowStepID,
		&sr.StepRef,
		&jobRunID,
		&sr.Status,
		&sr.DepsCompleted,
		&sr.DepsRequired,
		&output,
		&stepRunError,
		&startedAt,
		&finishedAt,
		&attempt,
		&sr.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if jobRunID != nil {
		sr.JobRunID = *jobRunID
	}
	if output != nil {
		sr.Output = json.RawMessage(output)
	}
	if stepRunError != nil {
		sr.Error = *stepRunError
	}
	sr.StartedAt = startedAt
	sr.FinishedAt = finishedAt
	sr.Attempt = attempt

	return &sr, nil
}
