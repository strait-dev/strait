package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

func (q *Queries) ArchiveConsumedOutboxBatch(ctx context.Context, olderThan time.Duration, batchSize int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ArchiveConsumedOutboxBatch")
	defer span.End()

	cutoff := time.Now().Add(-olderThan)

	query := `
		WITH to_archive AS (
			SELECT id FROM enqueue_outbox
			WHERE consumed_at IS NOT NULL
			  AND consumed_at < $1
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		),
		archived AS (
			INSERT INTO enqueue_outbox_history (
				id, project_id, job_id, payload, metadata, idempotency_key,
				scheduled_at, priority, created_at, consumed_at, error,
				retry_of_outbox_id
			)
			SELECT
				id, project_id, job_id, payload, metadata, idempotency_key,
				scheduled_at, priority, created_at, consumed_at, error,
				retry_of_outbox_id
			FROM enqueue_outbox
			WHERE id IN (SELECT id FROM to_archive)
			ON CONFLICT (id) DO NOTHING
			RETURNING id
		)
		DELETE FROM enqueue_outbox WHERE id IN (SELECT id FROM archived)`

	tag, err := q.db.Exec(ctx, query, cutoff, batchSize)
	if err != nil {
		return 0, fmt.Errorf("archive consumed outbox batch: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) DeleteOutboxHistoryPastRetention(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteOutboxHistoryPastRetention")
	defer span.End()

	query := `
		DELETE FROM enqueue_outbox_history
		WHERE id IN (
			SELECT id FROM enqueue_outbox_history
			WHERE archived_at < $1
			LIMIT $2
		)`

	tag, err := q.db.Exec(ctx, query, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("delete outbox history past retention: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) EnsureOutboxHistoryPartitions(ctx context.Context, monthsAhead int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnsureOutboxHistoryPartitions")
	defer span.End()

	for i := 0; i <= monthsAhead; i++ {
		start := time.Now().AddDate(0, i, 0)
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		name := fmt.Sprintf("enqueue_outbox_history_p%04d_%02d", start.Year(), start.Month())

		query := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF enqueue_outbox_history FOR VALUES FROM ('%s') TO ('%s')`,
			name,
			start.Format("2006-01-02"),
			end.Format("2006-01-02"),
		)
		if _, err := q.db.Exec(ctx, query); err != nil {
			return fmt.Errorf("ensure outbox history partition %s: %w", name, err)
		}
	}
	return nil
}
