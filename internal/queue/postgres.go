package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

type PostgresQueue struct {
	db store.DBTX
}

func NewPostgresQueue(db store.DBTX) *PostgresQueue {
	return &PostgresQueue{db: db}
}

func (q *PostgresQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "queue.Enqueue")
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

	run.Status = domain.StatusQueued
	if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
		run.Status = domain.StatusDelayed
	}

	query := `
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23
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
		run.DebugMode,
		dbscan.NilIfEmptyString(run.ContinuationOf),
		run.LineageDepth,
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("enqueue run: %w", err)
	}

	return nil
}

func (q *PostgresQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "queue.Dequeue")
	defer span.End()

	query := fmt.Sprintf(`
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (
				j.max_concurrency IS NULL OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.job_id = jr.job_id
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency
			  )
			ORDER BY jr.priority DESC, jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
		          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth`, domain.StatusDequeued, domain.StatusQueued)

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("dequeue run: %w", err)
	}

	return run, nil
}

func (q *PostgresQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "queue.DequeueN")
	defer span.End()

	query := fmt.Sprintf(`
		WITH claimed AS (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (
				j.max_concurrency IS NULL OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.job_id = jr.job_id
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency
			  )
			ORDER BY jr.priority DESC, jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
			          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, domain.StatusDequeued)

	rows, err := q.db.Query(ctx, query, n)
	if err != nil {
		return nil, fmt.Errorf("dequeue runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
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

func (q *PostgresQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "queue.DequeueNByProject")
	defer span.End()

	query := fmt.Sprintf(`
		WITH claimed AS (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND jr.project_id = $2
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (
				j.max_concurrency IS NULL OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.job_id = jr.job_id
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency
			  )
			ORDER BY jr.priority DESC, jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
			          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, domain.StatusDequeued)

	rows, err := q.db.Query(ctx, query, n, projectID)
	if err != nil {
		return nil, fmt.Errorf("dequeue runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue runs by project scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue runs by project rows: %w", err)
	}

	return runs, nil
}
