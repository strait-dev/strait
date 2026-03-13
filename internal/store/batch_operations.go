package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateBatchOperation(ctx context.Context, op *domain.BatchOperation) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateBatchOperation")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		INSERT INTO batch_operations (id, project_id, job_id, item_count, created_by)
		VALUES ($1, $2, $3, $4, $5)
	`, op.ID, op.ProjectID, op.JobID, op.ItemCount, op.CreatedBy)
	if err != nil {
		return fmt.Errorf("create batch operation: %w", err)
	}
	return nil
}

func (q *Queries) FinalizeBatchOperation(ctx context.Context, batchID string, createdCount int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.FinalizeBatchOperation")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		UPDATE batch_operations
		SET created_count = $2, finished_at = NOW()
		WHERE id = $1
	`, batchID, createdCount)
	if err != nil {
		return fmt.Errorf("finalize batch operation: %w", err)
	}
	return nil
}

func (q *Queries) GetBatchOperation(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetBatchOperation")
	defer span.End()

	var op domain.BatchOperation
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, job_id, item_count, created_count, created_by, created_at, finished_at
		FROM batch_operations
		WHERE id = $1 AND project_id = $2
	`, batchID, projectID).Scan(
		&op.ID, &op.ProjectID, &op.JobID, &op.ItemCount,
		&op.CreatedCount, &op.CreatedBy, &op.CreatedAt, &op.FinishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get batch operation: %w", err)
	}
	return &op, nil
}

func (q *Queries) ListBatchOperations(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListBatchOperations")
	defer span.End()

	query := `SELECT id, project_id, job_id, item_count, created_count, created_by, created_at, finished_at
		FROM batch_operations WHERE project_id = $1`
	args := []any{projectID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list batch operations: %w", err)
	}
	defer rows.Close()

	var ops []domain.BatchOperation
	for rows.Next() {
		var op domain.BatchOperation
		if err := rows.Scan(&op.ID, &op.ProjectID, &op.JobID, &op.ItemCount,
			&op.CreatedCount, &op.CreatedBy, &op.CreatedAt, &op.FinishedAt); err != nil {
			return nil, fmt.Errorf("list batch operations scan: %w", err)
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
