package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

type PostgresQueue struct {
	db               store.DBTX
	priorityAging    bool
	statementTimeout time.Duration
}

type PostgresQueueOption func(*PostgresQueue)

func WithPriorityAging(enabled bool) PostgresQueueOption {
	return func(q *PostgresQueue) {
		q.priorityAging = enabled
	}
}

func WithStatementTimeout(d time.Duration) PostgresQueueOption {
	return func(q *PostgresQueue) {
		q.statementTimeout = d
	}
}

// NewPostgresQueue creates a new Postgres-backed job queue using SKIP LOCKED.
func NewPostgresQueue(db store.DBTX, opts ...PostgresQueueOption) *PostgresQueue {
	q := &PostgresQueue{db: db}
	for _, opt := range opts {
		if opt != nil {
			opt(q)
		}
	}
	return q
}

func (q *PostgresQueue) setStatementTimeout(ctx context.Context) {
	if q.statementTimeout > 0 {
		ms := int(q.statementTimeout.Milliseconds())
		_, _ = q.db.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms))
	}
}

func (q *PostgresQueue) dequeueOrderByClause() string {
	if q.priorityAging {
		return "jr.priority + EXTRACT(EPOCH FROM (NOW() - jr.created_at)) / 3600 DESC, jr.created_at ASC"
	}

	return "jr.priority DESC, jr.created_at ASC"
}

func (q *PostgresQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Enqueue")
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

	tagsJSON := []byte("{}")
	if len(run.Tags) > 0 {
		var marshalErr error
		tagsJSON, marshalErr = json.Marshal(run.Tags)
		if marshalErr != nil {
			return fmt.Errorf("enqueue run: marshal tags: %w", marshalErr)
		}
	}

	query := `
		WITH idempotency_check AS (
			SELECT 1 FROM job_runs
			WHERE job_id = $2
			  AND idempotency_key = $18
			  AND idempotency_key IS NOT NULL
			  AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
			LIMIT 1
		)
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth,
			tags, job_version_id, created_by, concurrency_key, batch_id
		)
		SELECT
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23,
			$24::jsonb, $25, $26, $27, $28
		WHERE NOT EXISTS (SELECT 1 FROM idempotency_check)
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
		tagsJSON,
		dbscan.NilIfEmptyString(run.JobVersionID),
		dbscan.NilIfEmptyString(run.CreatedBy),
		dbscan.NilIfEmptyString(run.ConcurrencyKey),
		dbscan.NilIfEmptyString(run.BatchID),
	).Scan(&run.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && run.IdempotencyKey != "" {
			return domain.ErrIdempotencyConflict
		}
		return fmt.Errorf("enqueue run: %w", err)
	}

	return nil
}

// CopyFromer is implemented by pgxpool.Pool and pgx.Conn.
type CopyFromer interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

var copyFromColumns = []string{
	"id", "job_id", "project_id", "status", "attempt", "payload", "result", "error",
	"triggered_by", "scheduled_at", "started_at", "finished_at", "heartbeat_at",
	"next_retry_at", "expires_at", "parent_run_id", "priority", "idempotency_key",
	"job_version", "workflow_step_run_id", "debug_mode", "continuation_of",
	"lineage_depth", "tags", "job_version_id", "created_by", "concurrency_key", "batch_id",
}

// EnqueueBatch inserts multiple runs using pgx.CopyFrom (COPY protocol) for
// high throughput. Requires the underlying db to implement CopyFromer (e.g.
// pgxpool.Pool). Sends pg_notify after insert to wake workers.
func (q *PostgresQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.EnqueueBatch")
	defer span.End()

	if len(runs) == 0 {
		return 0, nil
	}

	copier, ok := q.db.(CopyFromer)
	if !ok {
		return 0, fmt.Errorf("enqueue batch: underlying db does not support CopyFrom")
	}

	for _, run := range runs {
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
	}

	rows := make([][]any, len(runs))
	for i, run := range runs {
		tagsJSON := []byte("{}")
		if len(run.Tags) > 0 {
			var err error
			tagsJSON, err = json.Marshal(run.Tags)
			if err != nil {
				return 0, fmt.Errorf("enqueue batch: marshal tags for run %d: %w", i, err)
			}
		}

		rows[i] = []any{
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
			tagsJSON,
			dbscan.NilIfEmptyString(run.JobVersionID),
			dbscan.NilIfEmptyString(run.CreatedBy),
			dbscan.NilIfEmptyString(run.ConcurrencyKey),
			dbscan.NilIfEmptyString(run.BatchID),
		}
	}

	n, err := copier.CopyFrom(ctx, pgx.Identifier{"job_runs"}, copyFromColumns, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, fmt.Errorf("enqueue batch: copy from: %w", err)
	}

	// Wake workers via pg_notify.
	if n > 0 {
		if _, notifyErr := q.db.Exec(ctx, "SELECT pg_notify($1, $2)", QueueWakeChannel, fmt.Sprintf("%d", n)); notifyErr != nil {
			// Non-fatal: workers will pick up via polling.
			slog.Warn("enqueue batch: pg_notify failed", "error", notifyErr)
		}
	}

	return n, nil
}

func (q *PostgresQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Dequeue")
	defer span.End()

	q.setStatementTimeout(ctx)

	query := fmt.Sprintf(`
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND j.enabled = true
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
			  AND (
				j.max_concurrency_per_key IS NULL
				OR jr.concurrency_key IS NULL
				OR jr.concurrency_key = ''
				OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.project_id = jr.project_id
					  AND active.concurrency_key = jr.concurrency_key
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency_per_key
			  )
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
		          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key`, domain.StatusDequeued, domain.StatusQueued, q.dequeueOrderByClause())

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil run signals empty queue.
		}
		return nil, fmt.Errorf("dequeue run: %w", err)
	}

	return run, nil
}

func (q *PostgresQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueN")
	defer span.End()

	q.setStatementTimeout(ctx)

	orderBy := q.dequeueOrderByClause()

	query := fmt.Sprintf(`
		WITH claimed AS (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND j.enabled = true
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
			  AND (
				j.max_concurrency_per_key IS NULL
				OR jr.concurrency_key IS NULL
				OR jr.concurrency_key = ''
				OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.project_id = jr.project_id
					  AND active.concurrency_key = jr.concurrency_key
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency_per_key
			  )
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
			          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, orderBy, domain.StatusDequeued)

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
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNByProject")
	defer span.End()

	q.setStatementTimeout(ctx)

	orderBy := q.dequeueOrderByClause()

	query := fmt.Sprintf(`
		WITH claimed AS (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND j.enabled = true
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
			  AND (
				j.max_concurrency_per_key IS NULL
				OR jr.concurrency_key IS NULL
				OR jr.concurrency_key = ''
				OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.project_id = jr.project_id
					  AND active.concurrency_key = jr.concurrency_key
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency_per_key
			  )
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
			          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, orderBy, domain.StatusDequeued)

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
