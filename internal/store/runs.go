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
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateRun(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	if run.Attempt == 0 {
		run.Attempt = 1
	}

	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}

	if run.Status == "" || run.Status == domain.StatusQueued {
		run.Status = domain.StatusQueued
		if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
			run.Status = domain.StatusDelayed
		}
	}

	query := `
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		run.ID,
		run.JobID,
		run.ProjectID,
		run.Status,
		run.Attempt,
		dbscan.NilIfEmptyRawMessage(run.Payload),
		dbscan.NilIfEmptyRawMessage(run.Result),
		dbscan.NilIfEmptyString(run.Error),
		run.TriggeredBy,
		run.ScheduledAt,
		run.StartedAt,
		run.FinishedAt,
		run.HeartbeatAt,
		run.NextRetryAt,
		run.ExpiresAt,
		dbscan.NilIfEmptyString(run.ParentRunID),
		run.Priority,
		dbscan.NilIfEmptyString(run.IdempotencyKey),
		run.JobVersion,
		dbscan.NilIfEmptyString(run.WorkflowStepRunID),
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}

	return nil
}

func (q *Queries) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetRun")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE id = $1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get run: %w", err)
	}

	return run, nil
}

func (q *Queries) GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetRunByIdempotencyKey")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE job_id = $1 AND idempotency_key = $2`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, jobID, idempotencyKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run by idempotency key: %w", err)
	}

	return run, nil
}

func (q *Queries) ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListRunsByJob")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE job_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := q.db.Query(ctx, query, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list runs by job: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by job scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by job rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
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
		return nil, fmt.Errorf("list runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by project scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by project rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateRunStatus")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("invalid status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"attempt":              {},
		"payload":              {},
		"result":               {},
		"error":                {},
		"error_class":          {},
		"triggered_by":         {},
		"scheduled_at":         {},
		"started_at":           {},
		"finished_at":          {},
		"heartbeat_at":         {},
		"next_retry_at":        {},
		"expires_at":           {},
		"workflow_step_run_id": {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
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
		if key == "error" || key == "workflow_step_run_id" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE job_runs SET %s WHERE id = $2 AND status = $3",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
	}

	return nil
}

func (q *Queries) UpdateHeartbeat(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateHeartbeat")
	defer span.End()

	query := `UPDATE job_runs SET heartbeat_at = NOW() WHERE id = $1`

	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}

	return nil
}

func (q *Queries) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListStaleRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE status = '%s' AND heartbeat_at < NOW() - $1::interval
		ORDER BY heartbeat_at ASC`, domain.StatusExecuting)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListDueRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE status = '%s' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC`, domain.StatusDelayed)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list due runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list due runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListExpiredRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE status IN ('%s', '%s')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC`, domain.StatusDelayed, domain.StatusQueued)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list expired runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListChildRuns")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE parent_run_id = $1
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, parentRunID)
	if err != nil {
		return nil, fmt.Errorf("list child runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list child runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list child runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListStaleDequeued")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id
		FROM job_runs
		WHERE status = '%s' AND started_at < NOW() - $1::interval
		ORDER BY started_at ASC`, domain.StatusDequeued)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale dequeued runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale dequeued runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale dequeued runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteTerminalRunsPastRetention")
	defer span.End()

	shortCutoff := time.Now().Add(-shortRetention)
	longCutoff := time.Now().Add(-longRetention)

	query := `
		DELETE FROM job_runs
		WHERE finished_at IS NOT NULL
		  AND (
			(status IN ('completed', 'failed', 'canceled', 'expired') AND finished_at <= $1)
			OR
			(status IN ('timed_out', 'crashed', 'system_failed') AND finished_at <= $2)
		  )`

	tag, err := q.db.Exec(ctx, query, shortCutoff, longCutoff)
	if err != nil {
		return 0, fmt.Errorf("delete terminal runs past retention: %w", err)
	}

	return tag.RowsAffected(), nil
}
