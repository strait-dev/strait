package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

// Outbox reader used by the scheduler flusher.

// OutboxRow is a single unconsumed enqueue_outbox row.
type OutboxRow struct {
	ID             string
	ProjectID      string
	JobID          string
	Payload        json.RawMessage
	Metadata       json.RawMessage
	IdempotencyKey *string
	ScheduledAt    *time.Time
	Priority       int
	CreatedAt      time.Time
}

// QuarantinedOutboxRow is a terminal outbox row kept for operator inspection.
type QuarantinedOutboxRow struct {
	ID             string
	ProjectID      string
	JobID          string
	Payload        json.RawMessage
	Metadata       json.RawMessage
	IdempotencyKey *string
	ScheduledAt    *time.Time
	Priority       int
	CreatedAt      time.Time
	ConsumedAt     time.Time
	Error          string
}

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

// claimOutboxOnConn is the shared implementation; accepts anything with
// a Query method so both *Queries.db and pgx.Tx work.
func claimOutboxOnConn(ctx context.Context, q outboxQuerier, limit int) ([]OutboxRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := q.Query(ctx, `
		SELECT id, project_id, job_id, payload, metadata,
		       idempotency_key, scheduled_at, priority, created_at
		FROM enqueue_outbox
		WHERE consumed_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
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
			&r.IdempotencyKey, &r.ScheduledAt, &r.Priority, &r.CreatedAt,
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
		       scheduled_at, priority, created_at, consumed_at, error
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
		       scheduled_at, priority, created_at, consumed_at, error
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
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOutboxRowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get quarantined outbox: %w", err)
	}
	return &row, nil
}
