package store

import (
	"context"
	"fmt"

	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) InsertBatchBufferItem(ctx context.Context, item *domain.BatchBufferItem) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.InsertBatchBufferItem")
	defer span.End()

	if item.ID == "" {
		item.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO batch_buffer (id, job_id, project_id, batch_key, payload, tags, priority, triggered_by, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		item.ID, item.JobID, item.ProjectID, item.BatchKey, item.Payload, item.Tags,
		item.Priority, item.TriggeredBy, nilIfEmpty(item.CreatedBy),
	).Scan(&item.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert batch buffer item: %w", err)
	}
	return nil
}

func (q *Queries) CountBatchBufferItems(ctx context.Context, jobID, batchKey string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountBatchBufferItems")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM batch_buffer WHERE job_id = $1 AND batch_key = $2`,
		jobID, batchKey,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count batch buffer items: %w", err)
	}
	return count, nil
}

func (q *Queries) DrainBatchBuffer(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DrainBatchBuffer")
	defer span.End()

	query := `
		WITH drained AS (
			DELETE FROM batch_buffer
			WHERE id IN (
				SELECT id FROM batch_buffer
				WHERE job_id = $1 AND batch_key = $2
				ORDER BY created_at ASC
				LIMIT $3
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, job_id, project_id, batch_key, payload, tags, priority, triggered_by, created_by, created_at
		)
		SELECT * FROM drained ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, jobID, batchKey, limit)
	if err != nil {
		return nil, fmt.Errorf("drain batch buffer: %w", err)
	}
	defer rows.Close()

	items := make([]domain.BatchBufferItem, 0, limit)
	for rows.Next() {
		var item domain.BatchBufferItem
		var createdBy *string
		if err := rows.Scan(
			&item.ID, &item.JobID, &item.ProjectID, &item.BatchKey,
			&item.Payload, &item.Tags, &item.Priority, &item.TriggeredBy,
			&createdBy, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("drain batch buffer scan: %w", err)
		}
		if createdBy != nil {
			item.CreatedBy = *createdBy
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) DrainBatchBufferInTx(ctx context.Context, tx DBTX, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
	return q.withDB(tx).DrainBatchBuffer(ctx, jobID, batchKey, limit)
}

func (q *Queries) ListBatchBufferItems(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListBatchBufferItems")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, job_id, project_id, batch_key, payload, tags, priority, triggered_by, created_by, created_at
		FROM batch_buffer
		WHERE job_id = $1 AND batch_key = $2
		ORDER BY created_at ASC
		LIMIT $3`, jobID, batchKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list batch buffer items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.BatchBufferItem, 0, limit)
	for rows.Next() {
		var item domain.BatchBufferItem
		var createdBy *string
		if err := rows.Scan(
			&item.ID, &item.JobID, &item.ProjectID, &item.BatchKey,
			&item.Payload, &item.Tags, &item.Priority, &item.TriggeredBy,
			&createdBy, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list batch buffer items scan: %w", err)
		}
		if createdBy != nil {
			item.CreatedBy = *createdBy
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) DeleteBatchBufferItems(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteBatchBufferItems")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM batch_buffer WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("delete batch buffer items: %w", err)
	}
	return nil
}

func (q *Queries) ListFlushableBatches(ctx context.Context) ([]FlushableBatch, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListFlushableBatches")
	defer span.End()

	query := `
		SELECT bb.job_id, bb.project_id, bb.batch_key, COUNT(*) AS item_count, MIN(bb.created_at) AS oldest_at
		FROM batch_buffer bb
		JOIN jobs j ON j.id = bb.job_id
		WHERE j.batch_window_secs > 0 OR j.batch_max_size > 0
		GROUP BY bb.job_id, bb.project_id, bb.batch_key, j.batch_window_secs, j.batch_max_size
		HAVING (j.batch_max_size > 0 AND COUNT(*) >= j.batch_max_size)
		   OR (j.batch_window_secs > 0 AND MIN(bb.created_at) + (j.batch_window_secs || ' seconds')::interval <= NOW())
		LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list flushable batches: %w", err)
	}
	defer rows.Close()

	batches := make([]FlushableBatch, 0, 16)
	for rows.Next() {
		var b FlushableBatch
		if err := rows.Scan(&b.JobID, &b.ProjectID, &b.BatchKey, &b.ItemCount, &b.OldestAt); err != nil {
			return nil, fmt.Errorf("list flushable batches scan: %w", err)
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}
