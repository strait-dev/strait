package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

type PostgresRunWriter struct {
	db               store.DBTX
	statementTimeout time.Duration
	metrics          *QueueMetrics
	backpressure     *Backpressure
}

type PostgresRunWriterOption func(*PostgresRunWriter)

func WithStatementTimeout(d time.Duration) PostgresRunWriterOption {
	return func(q *PostgresRunWriter) {
		q.statementTimeout = d
	}
}

// WithBackpressureController attaches a backpressure controller so
// EnqueueBatch consults the token bucket before inserting rows.
func WithBackpressureController(bp *Backpressure) PostgresRunWriterOption {
	return func(q *PostgresRunWriter) {
		q.backpressure = bp
	}
}

// NewPostgresRunWriter creates the Postgres run writer used by PgQue.
func NewPostgresRunWriter(db store.DBTX, opts ...PostgresRunWriterOption) *PostgresRunWriter {
	m, _ := Metrics()
	q := &PostgresRunWriter{db: db, metrics: m}
	for _, opt := range opts {
		if opt != nil {
			opt(q)
		}
	}
	return q
}

func (q *PostgresRunWriter) Enqueue(ctx context.Context, run *domain.JobRun) error {
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
func (q *PostgresRunWriter) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
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
func (q *PostgresRunWriter) prepareEnqueue(run *domain.JobRun) (string, []any, error) {
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

	query := enqueueRunInsertSQL(run.IdempotencyKey != "")

	execMode := run.ExecutionMode
	if execMode == "" {
		execMode = domain.ExecutionModeHTTP
	}
	queueName := runQueueName(run.QueueName)

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
		queueName,
		metadataJSON,
		run.IsRollback,
	}

	return query, args, nil
}

func enqueueRunInsertSQL(withIdempotency bool) string {
	if !withIdempotency {
		return `
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth,
			tags, job_version_id, created_by, concurrency_key, batch_id,
			execution_mode, queue_name, metadata,
			is_rollback
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23,
			$24::jsonb, $25, $26, $27, $28,
			$29, $30, $31::jsonb,
			$32
		)
		RETURNING created_at`
	}

	return `
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
			execution_mode, queue_name, metadata,
			is_rollback
		)
		SELECT
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23,
			$24::jsonb, $25, $26, $27, $28,
			$29, $30, $31::jsonb,
			$32
		WHERE NOT EXISTS (SELECT 1 FROM idempotency_check)
		RETURNING created_at`
}

// enqueueInManagedTx runs the enqueue inside a transaction when either
// idempotency locking or backpressure accounting must commit atomically with the
// row insert.
func (q *PostgresRunWriter) enqueueInManagedTx(
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

	if run.IdempotencyKey != "" {
		if err := q.acquireIdempotencyXactLock(ctx, tx, run.JobID, run.IdempotencyKey, "enqueue run"); err != nil {
			return err
		}
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

func (q *PostgresRunWriter) consumeBackpressure(ctx context.Context, db store.DBTX, run *domain.JobRun, op string) error {
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

func (q *PostgresRunWriter) acquireIdempotencyXactLock(ctx context.Context, db store.DBTX, jobID, idempotencyKey, op string) error {
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

func (q *PostgresRunWriter) insertPreparedRun(
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

	// This writer only persists job_runs. PgQue owns ready-event emission and
	// claim ownership around this insert.

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
	"execution_mode", "queue_name", "metadata",
	"is_rollback",
}

var emptyJSONB = []byte("{}")

// EnqueueBatch inserts multiple runs using pgx.CopyFrom (COPY protocol) for
// high throughput. Requires the underlying db to implement CopyFromer (e.g.
// pgxpool.Pool). PgQue emits ready events after this writer persists rows.
func (q *PostgresRunWriter) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.EnqueueBatch")
	defer span.End()

	if len(runs) == 0 {
		return 0, nil
	}

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

	now := time.Now()
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
		if run.ExecutionMode == "" {
			run.ExecutionMode = domain.ExecutionModeHTTP
		}
		run.Status = domain.StatusQueued
		if run.ScheduledAt != nil && run.ScheduledAt.After(now) {
			run.Status = domain.StatusDelayed
		}
	}

	n, err := copier.CopyFrom(ctx, pgx.Identifier{"job_runs"}, copyFromColumns, newJobRunCopyFromSource(runs))
	if err != nil {
		return 0, fmt.Errorf("enqueue batch: copy from: %w", err)
	}

	// This writer only persists job_runs. PgQue owns ready-event emission and
	// claim ownership around this insert.

	return n, nil
}

type jobRunCopyFromSource struct {
	runs   []*domain.JobRun
	idx    int
	values []any
}

func newJobRunCopyFromSource(runs []*domain.JobRun) *jobRunCopyFromSource {
	return &jobRunCopyFromSource{
		runs:   runs,
		idx:    -1,
		values: make([]any, len(copyFromColumns)),
	}
}

func (s *jobRunCopyFromSource) Next() bool {
	s.idx++
	return s.idx < len(s.runs)
}

func (s *jobRunCopyFromSource) Values() ([]any, error) {
	run := s.runs[s.idx]

	tagsJSON := emptyJSONB
	if len(run.Tags) > 0 {
		var err error
		tagsJSON, err = json.Marshal(run.Tags)
		if err != nil {
			return nil, fmt.Errorf("marshal tags for run %d: %w", s.idx, err)
		}
	}

	metadataJSON := emptyJSONB
	if len(run.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(run.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata for run %d: %w", s.idx, err)
		}
	}

	values := s.values
	values[0] = run.ID
	values[1] = run.JobID
	values[2] = run.ProjectID
	values[3] = run.Status
	values[4] = run.Attempt
	values[5] = dbscan.NilIfEmptyRawMessage(run.Payload)
	values[6] = dbscan.NilIfEmptyRawMessage(run.Result)
	values[7] = dbscan.NilIfEmptyString(run.Error)
	values[8] = run.TriggeredBy
	values[9] = run.ScheduledAt
	values[10] = run.StartedAt
	values[11] = run.FinishedAt
	values[12] = run.HeartbeatAt
	values[13] = run.NextRetryAt
	values[14] = run.ExpiresAt
	values[15] = dbscan.NilIfEmptyString(run.ParentRunID)
	values[16] = run.Priority
	values[17] = dbscan.NilIfEmptyString(run.IdempotencyKey)
	values[18] = run.JobVersion
	values[19] = dbscan.NilIfEmptyString(run.WorkflowStepRunID)
	values[20] = run.DebugMode
	values[21] = dbscan.NilIfEmptyString(run.ContinuationOf)
	values[22] = run.LineageDepth
	values[23] = tagsJSON
	values[24] = dbscan.NilIfEmptyString(run.JobVersionID)
	values[25] = dbscan.NilIfEmptyString(run.CreatedBy)
	values[26] = dbscan.NilIfEmptyString(run.ConcurrencyKey)
	values[27] = dbscan.NilIfEmptyString(run.BatchID)
	values[28] = string(run.ExecutionMode)
	values[29] = runQueueName(run.QueueName)
	values[30] = metadataJSON
	values[31] = run.IsRollback
	return values, nil
}

func (s *jobRunCopyFromSource) Err() error {
	return nil
}

func runQueueName(queueName string) string {
	if queueName == "" {
		return "default"
	}
	return queueName
}

// recordClaimMetrics samples the observed queue lag and retry schedule lag
// for the claimed run. Called from every successful dequeue variant.
func (q *PostgresRunWriter) recordClaimMetrics(ctx context.Context, run *domain.JobRun) {
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
