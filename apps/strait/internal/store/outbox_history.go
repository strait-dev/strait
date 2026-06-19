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
			SELECT eo.id,
			       COALESCE(eo.consumed_at, oc.updated_at, NOW()) AS archive_consumed_at
			FROM enqueue_outbox eo
			LEFT JOIN outbox_claims oc ON oc.outbox_id = eo.id
			WHERE (eo.error IS NULL OR eo.error = '')
			  AND (
			      (eo.consumed_at IS NOT NULL AND eo.consumed_at < $1)
			      OR (
			          eo.consumed_at IS NULL
			          AND oc.status = 'acked'
			          AND oc.updated_at <= $1
			      )
			  )
			ORDER BY COALESCE(eo.consumed_at, oc.updated_at, eo.created_at) ASC, eo.id ASC
			LIMIT $2
			FOR UPDATE OF eo SKIP LOCKED
		),
		archived AS (
			INSERT INTO enqueue_outbox_history (
				id, project_id, job_id, payload, metadata, idempotency_key,
				scheduled_at, priority, created_at, consumed_at, error,
				retry_of_outbox_id
			)
			SELECT
				eo.id, eo.project_id, eo.job_id, eo.payload, eo.metadata, eo.idempotency_key,
				eo.scheduled_at, eo.priority, eo.created_at, ta.archive_consumed_at, eo.error,
				eo.retry_of_outbox_id
			FROM enqueue_outbox eo
			JOIN to_archive ta ON ta.id = eo.id
			ON CONFLICT (id, consumed_at) DO NOTHING
			RETURNING id
		),
		deleted AS (
			DELETE FROM enqueue_outbox
			WHERE id IN (SELECT id FROM archived)
			RETURNING id
		),
		deleted_claims AS (
			DELETE FROM outbox_claims
			WHERE outbox_id IN (SELECT id FROM deleted)
			RETURNING outbox_id
		)
		SELECT COUNT(*), COALESCE((SELECT COUNT(*) FROM deleted_claims), 0) FROM deleted`

	var archived int64
	var deletedClaims int64
	if err := q.db.QueryRow(ctx, query, cutoff, batchSize).Scan(&archived, &deletedClaims); err != nil {
		return 0, fmt.Errorf("archive consumed outbox batch: %w", err)
	}
	return archived, nil
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

		quoted := `"` + name + `"`
		query := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF enqueue_outbox_history FOR VALUES FROM ('%s') TO ('%s')`,
			quoted,
			start.Format("2006-01-02"),
			end.Format("2006-01-02"),
		)
		if _, err := q.db.Exec(ctx, query); err != nil {
			return fmt.Errorf("ensure outbox history partition %s: %w", name, err)
		}
	}
	return nil
}
