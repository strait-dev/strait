package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain"

	"github.com/google/uuid"
)

func (q *Queries) CreateRun(ctx context.Context, run *domain.JobRun) error {
	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	if run.Attempt == 0 {
		run.Attempt = 1
	}

	if run.TriggeredBy == "" {
		run.TriggeredBy = "manual"
	}

	if run.Status == "" {
		run.Status = domain.StatusQueued
		if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
			run.Status = domain.StatusDelayed
		}
	}

	query := `
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16
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
		nilIfEmptyRawMessage(run.Payload),
		nilIfEmptyRawMessage(run.Result),
		nilIfEmptyString(run.Error),
		run.TriggeredBy,
		run.ScheduledAt,
		run.StartedAt,
		run.FinishedAt,
		run.HeartbeatAt,
		run.NextRetryAt,
		run.ExpiresAt,
		nilIfEmptyString(run.ParentRunID),
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}

	return nil
}

func (q *Queries) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
		FROM job_runs
		WHERE id = $1`

	run, err := scanRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}

	return run, nil
}

func (q *Queries) ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error) {
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
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
		run, err := scanRun(rows)
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
	baseQuery := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
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
		run, err := scanRun(rows)
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
	if err := domain.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("invalid status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"attempt":       {},
		"payload":       {},
		"result":        {},
		"error":         {},
		"triggered_by":  {},
		"scheduled_at":  {},
		"started_at":    {},
		"finished_at":   {},
		"heartbeat_at":  {},
		"next_retry_at": {},
		"expires_at":    {},
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
			return fmt.Errorf("unsupported update field: %s", key)
		}

		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = nilIfEmptyRawMessage(raw)
		}
		if key == "error" {
			if text, ok := value.(string); ok {
				value = nilIfEmptyString(text)
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
		return fmt.Errorf("run status update conflict for id %s from %s", id, from)
	}

	return nil
}

func (q *Queries) UpdateHeartbeat(ctx context.Context, id string) error {
	query := `UPDATE job_runs SET heartbeat_at = NOW() WHERE id = $1`

	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("run not found: %s", id)
	}

	return nil
}

func (q *Queries) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
		FROM job_runs
		WHERE status = 'executing' AND heartbeat_at < NOW() - $1::interval
		ORDER BY heartbeat_at ASC`

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
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
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
		FROM job_runs
		WHERE status = 'delayed' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
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
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
		FROM job_runs
		WHERE status IN ('delayed', 'queued')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
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
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
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
		run, err := scanRun(rows)
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

type runScanTarget interface {
	Scan(dest ...any) error
}

func scanRun(scanner runScanTarget) (*domain.JobRun, error) {
	var run domain.JobRun
	var payload []byte
	var result []byte
	var runError *string
	var parentRunID *string

	err := scanner.Scan(
		&run.ID,
		&run.JobID,
		&run.ProjectID,
		&run.Status,
		&run.Attempt,
		&payload,
		&result,
		&runError,
		&run.TriggeredBy,
		&run.ScheduledAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.HeartbeatAt,
		&run.NextRetryAt,
		&run.ExpiresAt,
		&parentRunID,
		&run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		run.Payload = json.RawMessage(payload)
	}
	if result != nil {
		run.Result = json.RawMessage(result)
	}
	if runError != nil {
		run.Error = *runError
	}
	if parentRunID != nil {
		run.ParentRunID = *parentRunID
	}

	return &run, nil
}
