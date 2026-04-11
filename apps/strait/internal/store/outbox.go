package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// R3 Phase 8: outbox reader used by the scheduler flusher.

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

// ClaimUnconsumedOutbox fetches up to `limit` unconsumed outbox rows
// using FOR UPDATE SKIP LOCKED so multiple flushers are safe. The
// caller must mark the rows consumed (ConsumeOutbox) in the same
// transaction; uncommitted rows automatically return to the unclaimed
// pool on rollback.
func (q *Queries) ClaimUnconsumedOutbox(ctx context.Context, limit int) ([]OutboxRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimUnconsumedOutbox")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}
	rows, err := q.db.Query(ctx, `
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

// MarkOutboxConsumed sets consumed_at=NOW() for each id. Called by the
// flusher in the same transaction as the job_runs insert.
func (q *Queries) MarkOutboxConsumed(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkOutboxConsumed")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	_, err := q.db.Exec(ctx, `
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
