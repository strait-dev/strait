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
	statementTimeout time.Duration
	metrics          *QueueMetrics
	backpressure     *Backpressure
}

type PostgresQueueOption func(*PostgresQueue)

// WithPriorityAging is deprecated and now a no-op. Priority aging is handled
// by the scheduler.PriorityPromoter goroutine in Phase 4 instead of by a
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

func (q *PostgresQueue) setStatementTimeout(ctx context.Context) {
	if q.statementTimeout > 0 {
		ms := int(q.statementTimeout.Milliseconds())
		_, _ = q.db.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms))
	}
}

// dequeueOrderByClause always returns the static, index-servable ordering.
// Starvation prevention is now handled by scheduler.PriorityPromoter which
// bumps priority on aging rows; see Phase 4 in the reliability plan.
func (q *PostgresQueue) dequeueOrderByClause() string {
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

	metadataJSON := []byte("{}")
	if len(run.Metadata) > 0 {
		var marshalErr error
		metadataJSON, marshalErr = json.Marshal(run.Metadata)
		if marshalErr != nil {
			return fmt.Errorf("enqueue run: marshal metadata: %w", marshalErr)
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
		string(execMode),
		dbscan.NilIfEmptyString(run.MachineID),
		metadataJSON,
		dbscan.NilIfEmptyString(run.DeploymentID),
		dbscan.NilIfEmptyString(run.PinnedImageURI),
		dbscan.NilIfEmptyString(run.PinnedImageDigest),
		run.IsRollback,
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

// concurrencyCTEs pre-computes active run counts per job and per concurrency key,
// replacing correlated COUNT(*) subqueries that re-execute per candidate row.
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

	q.setStatementTimeout(ctx)

	query := fmt.Sprintf(`
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

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query))
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

// DequeueNFullyDenormalized is the R2 Phase 6 variant that drops the
// `JOIN jobs` entirely by reading enabled/paused/max_concurrency from the
// denormalized columns on job_runs. The fan-out trigger on jobs keeps the
// columns current for non-terminal rows, so the dequeue hot path touches
// only job_runs + job_active_counts.
func (q *PostgresQueue) DequeueNFullyDenormalized(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNFullyDenormalized")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)

	query := fmt.Sprintf(`
		WITH claimed AS (
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
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, q.dequeueOrderByClause(), domain.StatusDequeued, dequeueColumns, dequeueColumns)

	rows, err := q.db.Query(ctx, query, n)
	if err != nil {
		return nil, fmt.Errorf("dequeue fully denormalized: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue fully denormalized scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue fully denormalized rows: %w", err)
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	return runs, nil
}

// DequeueNDenormalized is the Phase 6 variant that replaces the
// COUNT-over-active-rows CTE with a lookup against the job_active_counts
// table. The maintenance trigger guarantees the counter stays in sync with
// the job_runs status transitions, so the dequeue hot path does a single
// PK probe per candidate instead of scanning every in-flight row.
//
// Returns the same shape as DequeueN. Callers enable this variant via a
// feature flag at the executor layer.
func (q *PostgresQueue) DequeueNDenormalized(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNDenormalized")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)

	query := fmt.Sprintf(`
		WITH claimed AS (
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
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s
		FROM updated
		ORDER BY created_at ASC`, domain.StatusQueued, q.dequeueOrderByClause(), domain.StatusDequeued, dequeueColumns, dequeueColumns)

	rows, err := q.db.Query(ctx, query, n)
	if err != nil {
		return nil, fmt.Errorf("dequeue denormalized: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue denormalized scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue denormalized rows: %w", err)
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	return runs, nil
}

// DequeueNWithCursor is the Phase 5 cursor-aware variant. When cursor is
// non-nil and has a valid snapshot, its (created_at, id) pair is added to
// the claim predicate so Postgres can skip past already-visited heap tuples
// during B-tree descent. On empty result (no runs claimable beyond the
// cursor) the cursor is reset so older rows remain reachable.
func (q *PostgresQueue) DequeueNWithCursor(ctx context.Context, n int, cursor *ClaimCursor) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueN")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)

	orderBy := q.dequeueOrderByClause()

	cursorCreated, cursorID, cursorValid := cursor.Snapshot()
	args := []any{n}
	cursorClause := ""
	if cursorValid {
		cursorClause = "AND (jr.created_at, jr.id) > ($2, $3)"
		args = append(args, cursorCreated, cursorID)
	}

	query := fmt.Sprintf(`
		WITH %s,
		claimed AS (
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
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s
		FROM updated
		ORDER BY created_at ASC`, concurrencyCTEs, concurrencyJoins, domain.StatusQueued, concurrencyWhere, cursorClause, orderBy, domain.StatusDequeued, dequeueColumns, dequeueColumns)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		cursor.Reset()
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

	if len(runs) == 0 {
		// Empty claim => reset cursor so older rows are reachable next tick.
		cursor.Reset()
	} else {
		// Advance cursor to the max (created_at, id) we observed.
		for i := range runs {
			cursor.Advance(runs[i].CreatedAt, runs[i].ID)
		}
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}

	return runs, nil
}

// DequeueNFair dequeues up to n runs using fair round-robin across jobs.
// It picks at most one run per job before cycling, preventing high-volume
// jobs from starving others. Falls back to priority ordering within the
// fair selection.
func (q *PostgresQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNFair")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)

	orderBy := q.dequeueOrderByClause()

	query := fmt.Sprintf(`
		WITH %s,
		candidates AS (
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
			ORDER BY jr.job_id, %s
		), claimed AS (
			SELECT c.id
			FROM candidates c
			JOIN job_runs jr ON jr.id = c.id
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s
		FROM updated
		ORDER BY created_at ASC`, concurrencyCTEs, concurrencyJoins, domain.StatusQueued, concurrencyWhere, orderBy, orderBy, domain.StatusDequeued, dequeueColumns, dequeueColumns)

	rows, err := q.db.Query(ctx, query, n)
	if err != nil {
		return nil, fmt.Errorf("dequeue runs fair: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue runs fair scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue runs fair rows: %w", err)
	}

	return runs, nil
}

// DequeueNPartitioned claims up to n runs across the given project IDs
// in a single round trip. Uses DISTINCT ON (project_id) for fair
// scheduling so no single project can starve the others. Replaces the
// N-round-trip loop in executor_poll.dequeueAcrossPartitions.
func (q *PostgresQueue) DequeueNPartitioned(ctx context.Context, n int, projectIDs []string) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNPartitioned")
	defer span.End()

	if n <= 0 || len(projectIDs) == 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)
	orderBy := q.dequeueOrderByClause()

	query := fmt.Sprintf(`
		WITH %s,
		candidates AS (
			SELECT DISTINCT ON (jr.project_id) jr.id
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
			ORDER BY jr.project_id, %s
			FOR UPDATE OF jr SKIP LOCKED
		),
		claimed AS (
			SELECT id FROM candidates LIMIT $1
		),
		updated AS (
			UPDATE job_runs SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s FROM updated ORDER BY created_at ASC`,
		concurrencyCTEs, concurrencyJoins, domain.StatusQueued,
		concurrencyWhere, orderBy,
		domain.StatusDequeued, dequeueColumns, dequeueColumns,
	)

	rows, err := q.db.Query(ctx, query, n, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("dequeue partitioned: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("dequeue partitioned scan: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue partitioned rows: %w", err)
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	return runs, nil
}

func (q *PostgresQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.DequeueNByProject")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	q.setStatementTimeout(ctx)

	orderBy := q.dequeueOrderByClause()

	query := fmt.Sprintf(`
		WITH %s,
		claimed AS (
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
			LIMIT $1
		), updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s
		FROM updated
		ORDER BY created_at ASC`, concurrencyCTEs, concurrencyJoins, domain.StatusQueued, concurrencyWhere, orderBy, domain.StatusDequeued, dequeueColumns, dequeueColumns)

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
