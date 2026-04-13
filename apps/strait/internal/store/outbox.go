package store

import (
	"context"
	"encoding/json"
	"fmt"
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
