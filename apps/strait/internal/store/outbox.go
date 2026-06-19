package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

// Outbox reader used by the scheduler flusher.

// OutboxRow is a single unconsumed enqueue_outbox row.
type OutboxRow struct {
	ID              string
	ProjectID       string
	JobID           string
	Payload         json.RawMessage
	Metadata        json.RawMessage
	IdempotencyKey  *string
	ScheduledAt     *time.Time
	Priority        int
	CreatedAt       time.Time
	RetryOfOutboxID *string
	ExecutionMode   string
	QueueName       string
}

// QuarantinedOutboxRow is a terminal outbox row kept for operator inspection.
type QuarantinedOutboxRow struct {
	ID              string
	ProjectID       string
	JobID           string
	Payload         json.RawMessage
	Metadata        json.RawMessage
	IdempotencyKey  *string
	ScheduledAt     *time.Time
	Priority        int
	CreatedAt       time.Time
	ConsumedAt      time.Time
	Error           string
	RetryOfOutboxID *string
}

const claimOutboxSQL = `
	WITH candidates AS (
		SELECT oc.outbox_id
		FROM outbox_claims oc
		WHERE oc.status = 'ready'
		  AND EXISTS (
		      SELECT 1
		      FROM enqueue_outbox eo
		      WHERE eo.id = oc.outbox_id
		        AND eo.consumed_at IS NULL
		  )
		ORDER BY oc.created_at ASC, oc.outbox_id ASC
		FOR UPDATE OF oc SKIP LOCKED
		LIMIT $1
	),
	leased AS (
		UPDATE outbox_claims oc
		SET status = 'leased',
		    lease_owner = $2,
		    lease_expires_at = NOW() + $3,
		    claimed_at = NOW(),
		    attempts = attempts + 1,
		    updated_at = NOW()
		FROM candidates c
		WHERE oc.outbox_id = c.outbox_id
		RETURNING oc.outbox_id
	)
	SELECT eo.id, eo.project_id, eo.job_id, eo.payload, eo.metadata,
	       eo.idempotency_key, eo.scheduled_at, eo.priority, eo.created_at, eo.retry_of_outbox_id,
	       COALESCE(j.execution_mode, 'http'),
	       COALESCE(NULLIF(j.queue_name, ''), 'default')
	FROM enqueue_outbox eo
	JOIN leased l ON l.outbox_id = eo.id
	LEFT JOIN jobs j ON j.id = eo.job_id
	ORDER BY eo.created_at ASC
`

// ClaimUnconsumedOutbox fetches up to `limit` unconsumed outbox rows on
// the pool without a holding transaction. The caller must be aware that
// SKIP LOCKED releases locks as soon as the statement returns, so this
// variant is NOT safe for concurrent flushers. Use ClaimUnconsumedOutboxInTx
// with a pgx.Tx when running multiple flushers.
func (q *Queries) ClaimUnconsumedOutbox(ctx context.Context, limit int) ([]OutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimUnconsumedOutbox")
	defer span.End()
	return claimOutboxOnConn(ctx, q.db, limit)
}

// ClaimUnconsumedOutboxInTx is the safe-for-concurrent-flushers variant:
// the caller passes their own pgx.Tx, and FOR UPDATE SKIP LOCKED row
// locks are held for the duration of that transaction. Commit marks the
// claim durable; rollback returns the rows to the unclaimed pool.
func ClaimUnconsumedOutboxInTx(ctx context.Context, tx pgx.Tx, limit int) ([]OutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimUnconsumedOutboxInTx")
	defer span.End()
	return claimOutboxOnConn(ctx, tx, limit)
}

func ClaimOutboxInTx(ctx context.Context, tx pgx.Tx, limit int, leaseOwner string, leaseDuration time.Duration) ([]OutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimOutboxInTx")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	if leaseOwner == "" {
		leaseOwner = "outbox-flusher"
	}

	rows, err := tx.Query(ctx, claimOutboxSQL, limit, leaseOwner, leaseDuration)
	if err != nil {
		return nil, fmt.Errorf("claim outbox claim-log rows: %w", err)
	}
	defer rows.Close()

	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.JobID, &r.Payload, &r.Metadata,
			&r.IdempotencyKey, &r.ScheduledAt, &r.Priority, &r.CreatedAt, &r.RetryOfOutboxID,
			&r.ExecutionMode, &r.QueueName,
		); err != nil {
			return nil, fmt.Errorf("scan outbox claim-log row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func ReclaimExpiredOutboxClaimsInTx(ctx context.Context, tx pgx.Tx) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReclaimExpiredOutboxClaimsInTx")
	defer span.End()

	tag, err := tx.Exec(ctx, `
		UPDATE outbox_claims
		SET status = 'ready',
		    batch_id = NULL,
		    lease_owner = NULL,
		    lease_expires_at = NULL,
		    claimed_at = NULL,
		    updated_at = NOW()
		WHERE status = 'leased'
		  AND lease_expires_at <= NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("reclaim outbox claims: %w", err)
	}
	return tag.RowsAffected(), nil
}

func MarkOutboxClaimsReadyInTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE outbox_claims
		SET status = 'ready',
		    batch_id = NULL,
		    lease_owner = NULL,
		    lease_expires_at = NULL,
		    claimed_at = NULL,
		    updated_at = NOW()
		WHERE outbox_id = ANY($1)
	`, ids)
	if err != nil {
		return fmt.Errorf("mark outbox claims ready: %w", err)
	}
	return nil
}

func MarkOutboxClaimsAckedInTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE outbox_claims
		SET status = 'acked',
		    batch_id = NULL,
		    lease_owner = NULL,
		    lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE outbox_id = ANY($1)
	`, ids)
	if err != nil {
		return fmt.Errorf("mark outbox claims acked: %w", err)
	}
	return nil
}

// claimOutboxOnConn is the shared implementation; accepts anything with
// a Query method so both *Queries.db and pgx.Tx work.
func claimOutboxOnConn(ctx context.Context, q outboxQuerier, limit int) ([]OutboxRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := q.Query(ctx, `
		SELECT eo.id, eo.project_id, eo.job_id, eo.payload, eo.metadata,
		       eo.idempotency_key, eo.scheduled_at, eo.priority, eo.created_at, eo.retry_of_outbox_id,
		       COALESCE(j.execution_mode, 'http'),
		       COALESCE(NULLIF(j.queue_name, ''), 'default')
		FROM enqueue_outbox eo
		LEFT JOIN jobs j ON j.id = eo.job_id
		WHERE eo.consumed_at IS NULL
		ORDER BY eo.created_at ASC
		LIMIT $1
		FOR UPDATE OF eo SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.JobID, &r.Payload, &r.Metadata,
			&r.IdempotencyKey, &r.ScheduledAt, &r.Priority, &r.CreatedAt, &r.RetryOfOutboxID,
			&r.ExecutionMode, &r.QueueName,
		); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// outboxQuerier is the minimal surface both pgx.Tx and DBTX satisfy.
type outboxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// MarkOutboxConsumed sets consumed_at=NOW() for each id. Called by the
// flusher in the same transaction as the job_runs insert.
func (q *Queries) MarkOutboxConsumed(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkOutboxConsumed")
	defer span.End()
	return markOutboxOnExec(ctx, q.db, ids)
}

// MarkOutboxConsumedInTx marks outbox rows consumed within the caller's
// transaction. Used by the flusher pattern: Claim... (tx) -> enqueue ->
// MarkOutboxConsumedInTx (same tx) -> Commit.
func MarkOutboxConsumedInTx(ctx context.Context, tx pgx.Tx, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkOutboxConsumedInTx")
	defer span.End()
	return markOutboxOnExec(ctx, tx, ids)
}

type outboxExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func markOutboxOnExec(ctx context.Context, ex outboxExecer, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := ex.Exec(ctx, `
		UPDATE enqueue_outbox SET consumed_at = NOW() WHERE id = ANY($1) AND consumed_at IS NULL
	`, ids)
	if err != nil {
		return fmt.Errorf("mark outbox consumed: %w", err)
	}
	return nil
}

const maxOutboxErrorLength = 1024

// TruncateOutboxError normalizes operator-visible outbox error text before it
// is persisted or logged.
func TruncateOutboxError(errText string) string {
	msg := strings.TrimSpace(errText)
	if len(msg) > maxOutboxErrorLength {
		msg = msg[:maxOutboxErrorLength]
	}
	if msg == "" {
		msg = "outbox promotion failed"
	}
	return msg
}

// MarkOutboxErroredInTx records a terminal error on an outbox row and marks it
// consumed in the caller's transaction so it is no longer claimed again.
func MarkOutboxErroredInTx(ctx context.Context, tx pgx.Tx, id string, errText string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkOutboxErroredInTx")
	defer span.End()

	if id == "" {
		return nil
	}

	msg := TruncateOutboxError(errText)

	_, err := tx.Exec(ctx, `
		UPDATE enqueue_outbox
		SET error = $2, consumed_at = NOW()
		WHERE id = $1 AND consumed_at IS NULL
	`, id, msg)
	if err != nil {
		return fmt.Errorf("mark outbox errored: %w", err)
	}
	return nil
}

// CountUnconsumedOutbox returns the depth of the unconsumed outbox.
func (q *Queries) CountUnconsumedOutbox(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountUnconsumedOutbox")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM enqueue_outbox WHERE consumed_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count outbox: %w", err)
	}
	return count, nil
}

func (q *Queries) CountClaimableOutbox(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountClaimableOutbox")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_claims WHERE status = 'ready'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count claimable outbox claim-log rows: %w", err)
	}
	return count, nil
}

// OldestUnconsumedOutboxAge returns the age of the oldest unconsumed
// outbox row, or 0 if the table is empty. Used by the flusher metric.
func (q *Queries) OldestUnconsumedOutboxAge(ctx context.Context) (time.Duration, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.OldestUnconsumedOutboxAge")
	defer span.End()

	var age float64
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
		FROM enqueue_outbox WHERE consumed_at IS NULL
	`).Scan(&age)
	if err != nil {
		return 0, fmt.Errorf("oldest outbox age: %w", err)
	}
	return time.Duration(age * float64(time.Second)), nil
}

func (q *Queries) OldestClaimableOutboxAge(ctx context.Context) (time.Duration, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.OldestClaimableOutboxAge")
	defer span.End()

	var age float64
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
		FROM outbox_claims
		WHERE status = 'ready'
	`).Scan(&age)
	if err != nil {
		return 0, fmt.Errorf("oldest claimable outbox claim-log age: %w", err)
	}
	return time.Duration(age * float64(time.Second)), nil
}

// ListQuarantinedOutbox returns terminal outbox rows ordered by newest
// consumed_at/id first. Callers pass the previous page's last
// (consumed_at, id) tuple as the cursor to continue pagination.
func (q *Queries) ListQuarantinedOutbox(
	ctx context.Context,
	projectID string,
	limit int,
	cursorConsumedAt *time.Time,
	cursorID string,
) ([]QuarantinedOutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListQuarantinedOutbox")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	args := []any{projectID, limit}
	query := `
		SELECT id, project_id, job_id, payload, metadata, idempotency_key,
		       scheduled_at, priority, created_at, consumed_at, error, retry_of_outbox_id
		FROM enqueue_outbox
		WHERE project_id = $1
		  AND consumed_at IS NOT NULL
		  AND error IS NOT NULL
		  AND error <> ''
	`
	if cursorConsumedAt != nil {
		args = append(args, *cursorConsumedAt, cursorID)
		query += `
		  AND (consumed_at, id) < ($3, $4)
		`
	}
	query += `
		ORDER BY consumed_at DESC, id DESC
		LIMIT $2
	`

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list quarantined outbox: %w", err)
	}
	defer rows.Close()

	out := make([]QuarantinedOutboxRow, 0, limit)
	for rows.Next() {
		var row QuarantinedOutboxRow
		if err := rows.Scan(
			&row.ID,
			&row.ProjectID,
			&row.JobID,
			&row.Payload,
			&row.Metadata,
			&row.IdempotencyKey,
			&row.ScheduledAt,
			&row.Priority,
			&row.CreatedAt,
			&row.ConsumedAt,
			&row.Error,
			&row.RetryOfOutboxID,
		); err != nil {
			return nil, fmt.Errorf("scan quarantined outbox: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list quarantined outbox: %w", err)
	}
	return out, nil
}

// GetQuarantinedOutbox returns one terminal outbox row for operator inspection.
func (q *Queries) GetQuarantinedOutbox(ctx context.Context, projectID, id string) (*QuarantinedOutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetQuarantinedOutbox")
	defer span.End()

	var row QuarantinedOutboxRow
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, job_id, payload, metadata, idempotency_key,
		       scheduled_at, priority, created_at, consumed_at, error, retry_of_outbox_id
		FROM enqueue_outbox
		WHERE project_id = $1
		  AND id = $2
		  AND consumed_at IS NOT NULL
		  AND error IS NOT NULL
		  AND error <> ''
	`, projectID, id).Scan(
		&row.ID,
		&row.ProjectID,
		&row.JobID,
		&row.Payload,
		&row.Metadata,
		&row.IdempotencyKey,
		&row.ScheduledAt,
		&row.Priority,
		&row.CreatedAt,
		&row.ConsumedAt,
		&row.Error,
		&row.RetryOfOutboxID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOutboxRowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get quarantined outbox: %w", err)
	}
	return &row, nil
}

type outboxRowState struct {
	ID              string
	ProjectID       string
	JobID           string
	Payload         json.RawMessage
	Metadata        json.RawMessage
	IdempotencyKey  *string
	ScheduledAt     *time.Time
	Priority        int
	CreatedAt       time.Time
	ConsumedAt      *time.Time
	Error           *string
	RetryOfOutboxID *string
}

// RetryQuarantinedOutbox clones a quarantined row into a fresh, claimable
// outbox row while preserving the original row as immutable audit history.
func (q *Queries) RetryQuarantinedOutbox(ctx context.Context, projectID, id string) (*OutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RetryQuarantinedOutbox")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("retry quarantined outbox: db does not support transactions")
	}

	var cloned OutboxRow
	err := q.withTx(ctx, func(txQ *Queries) error {
		source, err := loadOutboxRowStateForUpdate(ctx, txQ.db, projectID, id)
		if err != nil {
			return err
		}
		if err := requireQuarantinedOutbox(source); err != nil {
			return err
		}

		var existingID string
		err = txQ.db.QueryRow(ctx, `
			SELECT id
			FROM enqueue_outbox
			WHERE retry_of_outbox_id = $1
			  AND consumed_at IS NULL
			LIMIT 1
		`, source.ID).Scan(&existingID)
		if err == nil {
			return fmt.Errorf("%w: active retry clone %s already exists for outbox row %s", ErrOutboxRowConflict, existingID, source.ID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("retry quarantined outbox: check active clone: %w", err)
		}

		cloned.ID = uuid.Must(uuid.NewV7()).String()
		err = txQ.db.QueryRow(ctx, `
			INSERT INTO enqueue_outbox (
				id, project_id, job_id, payload, metadata, idempotency_key,
				scheduled_at, priority, retry_of_outbox_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, project_id, job_id, payload, metadata, idempotency_key,
			          scheduled_at, priority, created_at, retry_of_outbox_id
		`,
			cloned.ID,
			source.ProjectID,
			source.JobID,
			source.Payload,
			source.Metadata,
			source.IdempotencyKey,
			source.ScheduledAt,
			source.Priority,
			source.ID,
		).Scan(
			&cloned.ID,
			&cloned.ProjectID,
			&cloned.JobID,
			&cloned.Payload,
			&cloned.Metadata,
			&cloned.IdempotencyKey,
			&cloned.ScheduledAt,
			&cloned.Priority,
			&cloned.CreatedAt,
			&cloned.RetryOfOutboxID,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%w: active retry clone already exists for outbox row %s", ErrOutboxRowConflict, source.ID)
			}
			return fmt.Errorf("retry quarantined outbox: insert clone: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &cloned, nil
}

// PurgeQuarantinedOutbox hard-deletes one quarantined row and returns the
// deleted snapshot for auditing.
func (q *Queries) PurgeQuarantinedOutbox(ctx context.Context, projectID, id string) (*QuarantinedOutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PurgeQuarantinedOutbox")
	defer span.End()

	var row QuarantinedOutboxRow
	err := q.db.QueryRow(ctx, `
		DELETE FROM enqueue_outbox
		WHERE project_id = $1
		  AND id = $2
		  AND consumed_at IS NOT NULL
		  AND error IS NOT NULL
		  AND error <> ''
		RETURNING id, project_id, job_id, payload, metadata, idempotency_key,
		          scheduled_at, priority, created_at, consumed_at, error, retry_of_outbox_id
	`, projectID, id).Scan(
		&row.ID,
		&row.ProjectID,
		&row.JobID,
		&row.Payload,
		&row.Metadata,
		&row.IdempotencyKey,
		&row.ScheduledAt,
		&row.Priority,
		&row.CreatedAt,
		&row.ConsumedAt,
		&row.Error,
		&row.RetryOfOutboxID,
	)
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("purge quarantined outbox: %w", err)
	}

	state, loadErr := loadOutboxRowState(ctx, q.db, projectID, id)
	if loadErr != nil {
		return nil, loadErr
	}
	if err := requireQuarantinedOutbox(state); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: outbox row %s disappeared before purge completed", ErrOutboxRowConflict, id)
}

func (q *Queries) PurgeQuarantinedOutboxOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PurgeQuarantinedOutboxOlderThan")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		DELETE FROM enqueue_outbox
		WHERE id IN (
			SELECT id FROM enqueue_outbox
			WHERE consumed_at IS NOT NULL
			  AND error IS NOT NULL AND error <> ''
			  AND consumed_at < $1
			LIMIT $2
		)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("purge stale quarantined outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

func loadOutboxRowState(ctx context.Context, db DBTX, projectID, id string) (*outboxRowState, error) {
	return scanOutboxRowState(
		ctx,
		db.QueryRow(ctx, `
			SELECT id, project_id, job_id, payload, metadata, idempotency_key,
			       scheduled_at, priority, created_at, consumed_at, error, retry_of_outbox_id
			FROM enqueue_outbox
			WHERE project_id = $1
			  AND id = $2
		`, projectID, id),
		projectID,
		id,
	)
}

func loadOutboxRowStateForUpdate(ctx context.Context, db DBTX, projectID, id string) (*outboxRowState, error) {
	return scanOutboxRowState(
		ctx,
		db.QueryRow(ctx, `
			SELECT id, project_id, job_id, payload, metadata, idempotency_key,
			       scheduled_at, priority, created_at, consumed_at, error, retry_of_outbox_id
			FROM enqueue_outbox
			WHERE project_id = $1
			  AND id = $2
			FOR UPDATE
		`, projectID, id),
		projectID,
		id,
	)
}

func scanOutboxRowState(ctx context.Context, row pgx.Row, projectID, id string) (*outboxRowState, error) {
	_ = ctx
	var state outboxRowState
	err := row.Scan(
		&state.ID,
		&state.ProjectID,
		&state.JobID,
		&state.Payload,
		&state.Metadata,
		&state.IdempotencyKey,
		&state.ScheduledAt,
		&state.Priority,
		&state.CreatedAt,
		&state.ConsumedAt,
		&state.Error,
		&state.RetryOfOutboxID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOutboxRowNotFound
		}
		return nil, fmt.Errorf("load outbox row %s/%s: %w", projectID, id, err)
	}
	return &state, nil
}

func requireQuarantinedOutbox(state *outboxRowState) error {
	if state == nil {
		return ErrOutboxRowNotFound
	}
	if state.ConsumedAt == nil || state.Error == nil || strings.TrimSpace(*state.Error) == "" {
		return fmt.Errorf("%w: outbox row %s is not quarantined", ErrOutboxRowConflict, state.ID)
	}
	return nil
}
