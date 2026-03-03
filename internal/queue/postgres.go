package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresQueue struct {
	db store.DBTX
}

func NewPostgresQueue(db store.DBTX) *PostgresQueue {
	return &PostgresQueue{db: db}
}

func (q *PostgresQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	if run.Attempt == 0 {
		run.Attempt = 1
	}

	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}

	run.Status = domain.StatusQueued
	if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
		run.Status = domain.StatusDelayed
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
		return fmt.Errorf("enqueue run: %w", err)
	}

	return nil
}

func (q *PostgresQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	query := `
		UPDATE job_runs
		SET status = 'dequeued', started_at = NOW()
		WHERE id = (
			SELECT id
			FROM job_runs
			WHERE status = 'queued'
			  AND (scheduled_at IS NULL OR scheduled_at <= NOW())
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, job_id, project_id, status, attempt, payload, result, error,
		          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		          next_retry_at, expires_at, parent_run_id, created_at`

	run, err := scanRun(q.db.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("dequeue run: %w", err)
	}

	return run, nil
}

func (q *PostgresQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	query := `
		WITH claimed AS (
			SELECT id
			FROM job_runs
			WHERE status = 'queued'
			  AND (scheduled_at IS NULL OR scheduled_at <= NOW())
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = 'dequeued', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING id, job_id, project_id, status, attempt, payload, result, error,
			          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			          next_retry_at, expires_at, parent_run_id, created_at
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, created_at
		FROM updated
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, n)
	if err != nil {
		return nil, fmt.Errorf("dequeue runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue runs rows: %w", err)
	}

	return runs, nil
}

type runScanner interface {
	Scan(dest ...any) error
}

func scanRun(scanner runScanner) (*domain.JobRun, error) {
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

func nilIfEmptyString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nilIfEmptyRawMessage(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
