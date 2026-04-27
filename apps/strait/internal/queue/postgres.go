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
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

type PostgresQueue struct {
	db               store.DBTX
	statementTimeout time.Duration
	metrics          *QueueMetrics
	backpressure     *Backpressure
}

type PostgresQueueOption func(*PostgresQueue)

// WithPriorityAging is deprecated and now a no-op. Priority aging is handled
// by the scheduler.PriorityPromoter goroutine instead of by a
// mutable ORDER BY expression, which had to sort over every queued row. The
// constructor option is kept so existing tests and call sites compile.
func WithPriorityAging(_ bool) PostgresQueueOption {
	return func(_ *PostgresQueue) {}
}

func WithStatementTimeout(d time.Duration) PostgresQueueOption {
	return func(q *PostgresQueue) {
		q.statementTimeout = d
	}
}

// WithBackpressureController attaches a backpressure controller so
// EnqueueBatch consults the token bucket before inserting rows.
func WithBackpressureController(bp *Backpressure) PostgresQueueOption {
	return func(q *PostgresQueue) {
		q.backpressure = bp
	}
}

// NewPostgresQueue creates a new Postgres-backed job queue using SKIP LOCKED.
func NewPostgresQueue(db store.DBTX, opts ...PostgresQueueOption) *PostgresQueue {
	m, _ := Metrics()
	q := &PostgresQueue{db: db, metrics: m}
	for _, opt := range opts {
		if opt != nil {
			opt(q)
		}
	}
	return q
}

// dequeueOrderByClause always returns the static, index-servable ordering.
// Starvation prevention is now handled by scheduler.PriorityPromoter which
// bumps priority on aging rows.
func (q *PostgresQueue) dequeueOrderByClause() string {
	return "jr.priority DESC, jr.created_at ASC"
}

func (q *PostgresQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Enqueue")
	defer span.End()

	query, args, err := q.prepareEnqueue(run)
	if err != nil {
		return err
	}

	needsManagedTx := run.IdempotencyKey != "" || q.backpressure != nil
	if needsManagedTx {
		if beginner, ok := q.db.(store.TxBeginner); ok {
			return q.enqueueInManagedTx(ctx, beginner, run, query, args)
		}
		if err := q.consumeBackpressure(ctx, q.db, run, "enqueue run"); err != nil {
			return err
		}
	}

	return q.insertPreparedRun(ctx, q.db, run, query, args, "enqueue run")
}

// EnqueueInTx inserts a run using the caller's transaction. When an
// idempotency key is present, it acquires the same transaction-scoped
// advisory lock used by Enqueue so concurrent transactional callers
// serialize on (job_id, idempotency_key) too.
func (q *PostgresQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.EnqueueInTx")
	defer span.End()

	query, args, err := q.prepareEnqueue(run)
	if err != nil {
		return err
	}

	if run.IdempotencyKey != "" {
		if err := q.acquireIdempotencyXactLock(ctx, tx, run.JobID, run.IdempotencyKey, "enqueue run in tx"); err != nil {
			return err
		}
	}
	if err := q.consumeBackpressure(ctx, tx, run, "enqueue run in tx"); err != nil {
		return err
	}

	return q.insertPreparedRun(ctx, tx, run, query, args, "enqueue run in tx")
}

// prepareEnqueue normalizes run fields and returns the INSERT query with
// bind args. Shared by Enqueue and EnqueueInTx.
func (q *PostgresQueue) prepareEnqueue(run *domain.JobRun) (string, []any, error) {
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
			return "", nil, fmt.Errorf("enqueue run: marshal tags: %w", marshalErr)
		}
	}

	metadataJSON := []byte("{}")
	if len(run.Metadata) > 0 {
		var marshalErr error
		metadataJSON, marshalErr = json.Marshal(run.Metadata)
		if marshalErr != nil {
			return "", nil, fmt.Errorf("enqueue run: marshal metadata: %w", marshalErr)
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
			tags, job_version_id, created_by, concurrency_key, batch_id,
			execution_mode, machine_id, metadata,
			deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		)
		SELECT
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23,
			$24::jsonb, $25, $26, $27, $28,
			$29, $30, $31::jsonb, $32, $33, $34, $35
		WHERE NOT EXISTS (SELECT 1 FROM idempotency_check)
		RETURNING created_at`

	execMode := run.ExecutionMode
	if execMode == "" {
		execMode = domain.ExecutionModeHTTP
	}

	args := []any{
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
		string(execMode),
		dbscan.NilIfEmptyString(run.MachineID),
		metadataJSON,
		dbscan.NilIfEmptyString(run.DeploymentID),
		dbscan.NilIfEmptyString(run.PinnedImageURI),
		dbscan.NilIfEmptyString(run.PinnedImageDigest),
		run.IsRollback,
	}

	return query, args, nil
}

// enqueueInManagedTx runs the enqueue inside a transaction when either
// idempotency locking or backpressure accounting must commit atomically with the
// row insert.
func (q *PostgresQueue) enqueueInManagedTx(
	ctx context.Context,
	beginner store.TxBeginner,
	run *domain.JobRun,
	query string,
	args []any,
) error {
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("enqueue run: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := q.acquireIdempotencyXactLock(ctx, tx, run.JobID, run.IdempotencyKey, "enqueue run"); err != nil {
		return err
	}

	if err := q.consumeBackpressure(ctx, tx, run, "enqueue run"); err != nil {
		return err
	}

	if err := q.insertPreparedRun(ctx, tx, run, query, args, "enqueue run"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("enqueue run: commit: %w", err)
	}
	return nil
}

func (q *PostgresQueue) consumeBackpressure(ctx context.Context, db store.DBTX, run *domain.JobRun, op string) error {
	if q.backpressure == nil {
		return nil
	}
	if err := q.backpressure.TryConsumeInTx(ctx, db, run.ProjectID); err != nil {
		if errors.Is(err, ErrEnqueueThrottled) {
			return err
		}
		return fmt.Errorf("%s: backpressure: %w", op, err)
	}
	return nil
}

func (q *PostgresQueue) acquireIdempotencyXactLock(ctx context.Context, db store.DBTX, jobID, idempotencyKey, op string) error {
	// hashtext returns int4 in Postgres; pg_advisory_xact_lock(int, int)
	// takes two int4 keys, which is the portable form we want (no int64
	// concatenation that could differ between little-/big-endian
	// arithmetic). The function returns void, so we use Exec.
	if _, err := db.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1)::int, hashtext($2)::int)`,
		jobID,
		idempotencyKey,
	); err != nil {
		return fmt.Errorf("%s: advisory lock: %w", op, err)
	}
	return nil
}

func (q *PostgresQueue) insertPreparedRun(
	ctx context.Context,
	db store.DBTX,
	run *domain.JobRun,
	query string,
	args []any,
	op string,
) error {
	if err := db.QueryRow(ctx, query, args...).Scan(&run.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && run.IdempotencyKey != "" {
			return domain.ErrIdempotencyConflict
		}
		if terminal := classifyTerminalEnqueueInsertError(err); terminal != nil {
			return &TerminalEnqueueError{
				Reason: terminal.reason,
				Err:    fmt.Errorf("%s: %w", op, err),
			}
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	// Note: claim rows for the dequeue hot path are created by the
	// trg_job_runs_claim_queue_sync trigger (migration 000224), which fires
	// on the job_runs INSERT above. No application-level dual-write needed.

	return nil
}

type terminalEnqueueReason struct {
	reason string
}

func classifyTerminalEnqueueInsertError(err error) *terminalEnqueueReason {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23503":
		return &terminalEnqueueReason{reason: "foreign_key_violation"}
	case "23502":
		return &terminalEnqueueReason{reason: "not_null_violation"}
	case "23514":
		return &terminalEnqueueReason{reason: "check_violation"}
	case "22P02":
		return &terminalEnqueueReason{reason: "invalid_text_representation"}
	}

	if len(pgErr.Code) >= 2 {
		switch pgErr.Code[:2] {
		case "22":
			return &terminalEnqueueReason{reason: "data_exception"}
		case "23":
			return &terminalEnqueueReason{reason: "integrity_constraint_violation"}
		}
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
	"execution_mode", "machine_id", "metadata",
	"deployment_id", "pinned_image_uri", "pinned_image_digest", "is_rollback",
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

	// R4 hardening: consult backpressure before inserting.
	if q.backpressure != nil && len(runs) > 0 {
		projectID := runs[0].ProjectID
		if err := q.backpressure.TryConsumeN(ctx, projectID, len(runs)); err != nil {
			return 0, err
		}
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

		metadataJSON := []byte("{}")
		if len(run.Metadata) > 0 {
			var err error
			metadataJSON, err = json.Marshal(run.Metadata)
			if err != nil {
				return 0, fmt.Errorf("enqueue batch: marshal metadata for run %d: %w", i, err)
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
			string(run.ExecutionMode),
			dbscan.NilIfEmptyString(run.MachineID),
			metadataJSON,
			dbscan.NilIfEmptyString(run.DeploymentID),
			dbscan.NilIfEmptyString(run.PinnedImageURI),
			dbscan.NilIfEmptyString(run.PinnedImageDigest),
			run.IsRollback,
		}
	}

	n, err := copier.CopyFrom(ctx, pgx.Identifier{"job_runs"}, copyFromColumns, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, fmt.Errorf("enqueue batch: copy from: %w", err)
	}

	// Note: claim rows for the dequeue hot path are created by the
	// trg_job_runs_claim_queue_sync trigger (migration 000224), which fires
	// on the COPY INSERT above. No application-level dual-write needed.
	// Wake workers via pg_notify.
	if n > 0 {
		if _, notifyErr := q.db.Exec(ctx, "SELECT pg_notify($1, $2)", QueueWakeChannel, fmt.Sprintf("%d", n)); notifyErr != nil {
			// Non-fatal: workers will pick up via polling.
			slog.Warn("enqueue batch: pg_notify failed", "error", notifyErr)
		}
	}

	return n, nil
}

// dequeueColumns is the shared column list for all dequeue RETURNING/SELECT clauses.
const dequeueColumns = `id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback, replayed_run_id`

// concurrencyCTEs is the fallback concurrency-checking path used when
// QUEUE_USE_DENORMALIZED_DEQUEUE is false. It scans all active runs
// per dequeue call (O(active_runs)). The default denormalized path
// uses job_active_counts for O(1) lookups instead.
//
// The supporting indexes (idx_job_runs_active_by_job and
// idx_job_runs_concurrency_key_active) were dropped in migration
// 000221. This CTE path still works -- Postgres will seq-scan the
// partition -- but performance degrades with many in-flight runs.
const concurrencyCTEs = `
active_by_job AS (
    SELECT job_id, COUNT(*) as cnt
    FROM job_runs
    WHERE status IN ('dequeued', 'executing')
    GROUP BY job_id
),
active_by_key AS (
    SELECT project_id, concurrency_key, COUNT(*) as cnt
    FROM job_runs
    WHERE status IN ('dequeued', 'executing')
    AND concurrency_key IS NOT NULL AND concurrency_key != ''
    GROUP BY project_id, concurrency_key
)`

const concurrencyJoins = `
    LEFT JOIN active_by_job abj ON abj.job_id = jr.job_id
    LEFT JOIN active_by_key abk ON abk.project_id = jr.project_id
        AND abk.concurrency_key = jr.concurrency_key`

const concurrencyWhere = `
    AND (j.max_concurrency IS NULL OR COALESCE(abj.cnt, 0) < j.max_concurrency)
    AND (j.max_concurrency_per_key IS NULL
         OR jr.concurrency_key IS NULL
         OR jr.concurrency_key = ''
         OR COALESCE(abk.cnt, 0) < j.max_concurrency_per_key)`

func (q *PostgresQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Dequeue")
	defer span.End()

	db := q.db
	var cleanup func()

	if q.statementTimeout > 0 {
		if beginner, ok := q.db.(store.TxBeginner); ok {
			tx, err := beginner.Begin(ctx)
			if err != nil {
				return nil, fmt.Errorf("dequeue run: begin tx: %w", err)
			}
			ms := int(q.statementTimeout.Milliseconds())
			if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)); err != nil {
				_ = tx.Rollback(ctx)
				return nil, fmt.Errorf("dequeue run: set statement timeout: %w", err)
			}
			db = tx
			cleanup = func() {
				_ = tx.Commit(ctx)
			}
		}
	}
	if cleanup != nil {
		defer cleanup()
	}

	query := "/* action=dequeue */ " + fmt.Sprintf(`
		WITH %s
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT 1
		)
		RETURNING %s`, concurrencyCTEs, domain.StatusDequeued, concurrencyJoins, domain.StatusQueued, concurrencyWhere, q.dequeueOrderByClause(), dequeueColumns)

	run, err := dbscan.ScanRun(db.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // nil run signals empty queue.
		}
		return nil, fmt.Errorf("dequeue run: %w", err)
	}

	q.recordClaimMetrics(ctx, run)
	return run, nil
}

// recordClaimMetrics samples the observed queue lag and retry schedule lag
// for the claimed run. Called from every successful dequeue variant.
func (q *PostgresQueue) recordClaimMetrics(ctx context.Context, run *domain.JobRun) {
	if q.metrics == nil || run == nil {
		return
	}
	if !run.CreatedAt.IsZero() {
		age := time.Since(run.CreatedAt).Seconds()
		if age >= 0 {
			q.metrics.OldestQueuedAge.Record(ctx, age)
		}
	}
	if run.NextRetryAt != nil && !run.NextRetryAt.IsZero() {
		lag := time.Since(*run.NextRetryAt).Seconds()
		if lag >= 0 {
			q.metrics.RetryScheduleLag.Record(ctx, lag)
		}
	}
}

func (q *PostgresQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	if n <= 0 {
		return nil, nil
	}
	return q.DequeueNWithCursor(ctx, n, nil)
}

// DequeueNFullyDenormalized is the fully-denormalized variant that drops the
// `JOIN jobs` entirely by reading enabled/paused/max_concurrency from the
// denormalized columns on job_runs. The fan-out trigger on jobs keeps the
// columns current for non-terminal rows, so the dequeue hot path touches
// only job_runs + job_active_counts.
func (q *PostgresQueue) DequeueNFullyDenormalized(ctx context.Context, n int) ([]domain.JobRun, error) {
	return executeDequeue(ctx, q, n, dequeueSpec{
		spanName:            "queue.DequeueNFullyDenormalized",
		skipConcurrencyCTEs: true,
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id, jr.created_at
			FROM job_runs jr
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = jr.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = jr.job_id
			  AND jac_key.concurrency_key = COALESCE(jr.concurrency_key, '')
			WHERE jr.status = '%s'
			  AND COALESCE(jr.job_enabled, true) = true
			  AND COALESCE(jr.job_paused, false) = false
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (jr.job_max_concurrency IS NULL OR COALESCE(jac_job.count, 0) < jr.job_max_concurrency)
			  AND (jr.job_max_concurrency_per_key IS NULL
			       OR jr.concurrency_key IS NULL
			       OR jr.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key)
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, domain.StatusQueued, q.dequeueOrderByClause()),
	})
}

// DequeueNTwoPhase is the two-phase variant that separates the B-tree scan
// from the fat-row fetch. Phase 1 claims IDs with a thin RETURNING id;
// phase 2 fetches the full 38-column rows by PK. This eliminates fat-row
// deserialization during the SKIP LOCKED scan, which is the dominant cost
// when dead tuples force repeated heap page reads.
func (q *PostgresQueue) DequeueNTwoPhase(ctx context.Context, n int) ([]domain.JobRun, error) {
	return executeDequeueTwoPhase(ctx, q, n, dequeueSpec{
		spanName:            "queue.DequeueNTwoPhase",
		skipConcurrencyCTEs: true,
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = jr.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = jr.job_id
			  AND jac_key.concurrency_key = COALESCE(jr.concurrency_key, '')
			WHERE jr.status = '%s'
			  AND COALESCE(jr.job_enabled, true) = true
			  AND COALESCE(jr.job_paused, false) = false
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (jr.job_max_concurrency IS NULL OR COALESCE(jac_job.count, 0) < jr.job_max_concurrency)
			  AND (jr.job_max_concurrency_per_key IS NULL
			       OR jr.concurrency_key IS NULL
			       OR jr.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key)
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, domain.StatusQueued, q.dequeueOrderByClause()),
	})
}

// DequeueNDenormalized is the denormalized variant that replaces the
// COUNT-over-active-rows CTE with a lookup against the job_active_counts
// table. The maintenance trigger guarantees the counter stays in sync with
// the job_runs status transitions, so the dequeue hot path does a single
// PK probe per candidate instead of scanning every in-flight row.
//
// Returns the same shape as DequeueN. Callers enable this variant via a
// feature flag at the executor layer.
func (q *PostgresQueue) DequeueNDenormalized(ctx context.Context, n int) ([]domain.JobRun, error) {
	return executeDequeue(ctx, q, n, dequeueSpec{
		spanName:            "queue.DequeueNDenormalized",
		skipConcurrencyCTEs: true,
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id, jr.created_at
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = jr.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = jr.job_id
			  AND jac_key.concurrency_key = COALESCE(jr.concurrency_key, '')
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (j.max_concurrency IS NULL OR COALESCE(jac_job.count, 0) < j.max_concurrency)
			  AND (j.max_concurrency_per_key IS NULL
			       OR jr.concurrency_key IS NULL
			       OR jr.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < j.max_concurrency_per_key)
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, domain.StatusQueued, q.dequeueOrderByClause()),
	})
}

// DequeueNWithCursor is the cursor-aware variant. When cursor is
// non-nil and has a valid snapshot, its (created_at, id) pair is added to
// the claim predicate so Postgres can skip past already-visited heap tuples
// during B-tree descent. On empty or partial result (fewer runs returned
// than requested) the cursor is reset so older rows -- retries, backdated
// runs -- remain reachable.
func (q *PostgresQueue) DequeueNWithCursor(ctx context.Context, n int, cursor *ClaimCursor) ([]domain.JobRun, error) {
	if n <= 0 {
		return nil, nil
	}

	orderBy := q.dequeueOrderByClause()

	cursorCreated, cursorID, cursorValid := cursor.Snapshot()
	var extraArgs []any
	cursorClause := ""
	if cursorValid {
		cursorClause = "AND (jr.created_at, jr.id) > ($2, $3)"
		extraArgs = append(extraArgs, cursorCreated, cursorID)
	}

	return executeDequeue(ctx, q, n, dequeueSpec{
		spanName: "queue.DequeueN",
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			  %s
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, concurrencyJoins, domain.StatusQueued, concurrencyWhere, cursorClause, orderBy),
		extraArgs: extraArgs,
		postScanFn: func(runs []domain.JobRun) error {
			if len(runs) < n {
				// Partial or empty result: reset cursor so retried runs,
				// backdated created_at, and next_retry_at rows that fall
				// behind the cursor position become reachable again.
				cursor.Reset()
			}
			for i := range runs {
				cursor.Advance(runs[i].CreatedAt, runs[i].ID)
			}
			return nil
		},
	})
}

// DequeueNFair dequeues up to n runs using fair round-robin across jobs.
// It picks at most one run per job before cycling, preventing high-volume
// jobs from starving others. Falls back to priority ordering within the
// fair selection.
func (q *PostgresQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	orderBy := q.dequeueOrderByClause()

	return executeDequeueFair(ctx, q, n, dequeueSpec{
		spanName: "queue.DequeueNFair",
		candidatesSQL: fmt.Sprintf(`
			SELECT DISTINCT ON (jr.job_id) jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			ORDER BY jr.job_id, %s`, concurrencyJoins, domain.StatusQueued, concurrencyWhere, orderBy),
	})
}

// DequeueNPartitioned claims up to n runs across the given project IDs
// in a single round trip. Uses DISTINCT ON (project_id) for fair
// scheduling so no single project can starve the others. Replaces the
// N-round-trip loop in executor_poll.dequeueAcrossPartitions.
func (q *PostgresQueue) DequeueNPartitioned(ctx context.Context, n int, projectIDs []string) ([]domain.JobRun, error) {
	if n <= 0 || len(projectIDs) == 0 {
		return nil, nil
	}

	orderBy := q.dequeueOrderByClause()

	return executeDequeue(ctx, q, n, dequeueSpec{
		spanName: "queue.DequeueNPartitioned",
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND jr.project_id = ANY($2)
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, concurrencyJoins, domain.StatusQueued, concurrencyWhere, orderBy),
		extraArgs: []any{projectIDs},
	})
}

func (q *PostgresQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	orderBy := q.dequeueOrderByClause()

	return executeDequeue(ctx, q, n, dequeueSpec{
		spanName: "queue.DequeueNByProject",
		candidatesSQL: fmt.Sprintf(`
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND NOT j.paused
			  AND jr.project_id = $2
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, concurrencyJoins, domain.StatusQueued, concurrencyWhere, orderBy),
		extraArgs: []any{projectID},
	})
}

// Claim table support.

// Pre-computed SQL for DequeueNClaim. Built once at package init to
// avoid fmt.Sprintf on every dequeue call.
var claimDeleteSQL = "/* action=dequeue */ " + `
	DELETE FROM job_run_queue
	WHERE run_id IN (
		SELECT q.run_id
		FROM job_run_queue q
		LEFT JOIN job_active_counts jac_job
		  ON jac_job.job_id = q.job_id AND jac_job.concurrency_key = ''
		LEFT JOIN job_active_counts jac_key
		  ON jac_key.job_id = q.job_id
		  AND jac_key.concurrency_key = COALESCE(q.concurrency_key, '')
		WHERE COALESCE(q.job_enabled, true) = true
		  AND COALESCE(q.job_paused, false) = false
		  AND (q.scheduled_at IS NULL OR q.scheduled_at <= NOW())
		  AND (q.next_retry_at IS NULL OR q.next_retry_at <= NOW())
		  AND (q.job_max_concurrency IS NULL
		       OR COALESCE(jac_job.count, 0) < q.job_max_concurrency)
		  AND (q.job_max_concurrency_per_key IS NULL
		       OR q.concurrency_key IS NULL
		       OR q.concurrency_key = ''
		       OR COALESCE(jac_key.count, 0) < q.job_max_concurrency_per_key)
		ORDER BY q.priority DESC, q.created_at ASC
		FOR UPDATE OF q SKIP LOCKED
		LIMIT $1
	)
	RETURNING run_id`

var claimUpdateFetchSQL = "/* action=dequeue */ " + fmt.Sprintf(`
	WITH claimed_update AS (
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = ANY($1)
		RETURNING %s
	)
	SELECT %s FROM claimed_update ORDER BY created_at ASC`,
	domain.StatusExecuting, dequeueColumns, dequeueColumns)

// claimInsertFromJobSQL inserts a claim row with the job's current
// enabled/paused/concurrency config. Uses a subquery against the jobs
// table so the claim row reflects reality at INSERT time, not hardcoded
// defaults that could be stale if the job was already paused/disabled.
const claimInsertFromJobSQL = `
	INSERT INTO job_run_queue (
		run_id, job_id, project_id, priority, created_at,
		scheduled_at, next_retry_at, concurrency_key,
		job_max_concurrency, job_max_concurrency_per_key,
		job_enabled, job_paused
	)
	SELECT $1, $2, $3, $4, $5, $6, $7, $8,
		j.max_concurrency, j.max_concurrency_per_key,
		j.enabled, j.paused
	FROM jobs j
	WHERE j.id = $2
	ON CONFLICT (run_id) DO NOTHING`

// InsertClaimRow inserts a thin claim row into job_run_queue with the
// job's current config. Called by Enqueue, EnqueueInTx, EnqueueBatch,
// retry re-enqueue, and delayed promotion.
func (q *PostgresQueue) InsertClaimRow(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	_, err := db.Exec(ctx, claimInsertFromJobSQL,
		run.ID, run.JobID, run.ProjectID, run.Priority, run.CreatedAt,
		run.ScheduledAt, run.NextRetryAt,
		dbscan.NilIfEmptyString(run.ConcurrencyKey),
	)
	if err != nil {
		return fmt.Errorf("insert claim row: %w", err)
	}
	return nil
}

// InsertClaimRowFromEnqueue inserts a claim row using the fields available
// at enqueue time (before the row is committed to job_runs). The created_at
// is NOW() since the job_runs INSERT hasn't returned it yet.
func (q *PostgresQueue) InsertClaimRowFromEnqueue(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	_, err := db.Exec(ctx, claimInsertFromJobSQL,
		run.ID, run.JobID, run.ProjectID, run.Priority, time.Now(),
		run.ScheduledAt, run.NextRetryAt,
		dbscan.NilIfEmptyString(run.ConcurrencyKey),
	)
	if err != nil {
		slog.Warn("insert claim row from enqueue failed", "error", err, "run_id", run.ID, "job_id", run.JobID)
		return nil
	}
	return nil
}

// DequeueNClaim is the claim-table dequeue variant. It DELETEs from the
// thin job_run_queue table (80 bytes/row), then UPDATEs job_runs status,
// then SELECTs the full rows by PK.
func (q *PostgresQueue) DequeueNClaim(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNClaim")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		return q.DequeueNTwoPhase(ctx, n)
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue claim: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if q.statementTimeout > 0 {
		ms := int(q.statementTimeout.Milliseconds())
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)); err != nil {
			return nil, fmt.Errorf("dequeue claim: set statement timeout: %w", err)
		}
	}

	// Phase 1: DELETE from thin claim table.
	rows, err := tx.Query(ctx, claimDeleteSQL, n)
	if err != nil {
		// If the claim table doesn't exist (pre-migration), rollback and
		// fall back to two-phase dequeue transparently.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
			_ = tx.Rollback(ctx)
			return q.DequeueNTwoPhase(ctx, n)
		}
		return nil, fmt.Errorf("dequeue claim: delete: %w", err)
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("dequeue claim: scan id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue claim: rows: %w", err)
	}

	if len(ids) == 0 {
		_ = tx.Commit(ctx)
		return nil, nil
	}

	// Phase 2: UPDATE status and fetch full rows in a single CTE.
	// Sets status directly to 'executing' (skipping the intermediate
	// 'dequeued' state) because the claim table DELETE already
	// represents "claimed by a worker". This eliminates one UPDATE
	// per run lifecycle = 33% fewer dead tuples on job_runs.
	fetchRows, err := tx.Query(ctx, claimUpdateFetchSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("dequeue claim: update+fetch: %w", err)
	}
	defer fetchRows.Close()

	runs := make([]domain.JobRun, 0, len(ids))
	for fetchRows.Next() {
		run, err := dbscan.ScanRun(fetchRows)
		if err != nil {
			return nil, fmt.Errorf("dequeue claim: fetch scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := fetchRows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue claim: fetch rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dequeue claim: commit: %w", err)
	}

	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	return runs, nil
}
