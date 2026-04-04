package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) AppendNotificationBatchEvent(ctx context.Context, batch *domain.NotificationBatch, event json.RawMessage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AppendNotificationBatchEvent")
	defer span.End()

	if batch.ID == "" {
		batch.ID = uuid.Must(uuid.NewV7()).String()
	}
	if batch.Status == "" {
		batch.Status = domain.NotifyBatchStatusCollecting
	}
	if len(event) == 0 {
		event = []byte(`{}`)
	}

	query := `
		INSERT INTO notification_batches (
			id, project_id, recipient_type, recipient_id, batch_key, channel, status, events, event_count, window_end
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, jsonb_build_array($8::jsonb), 1, $9
		)
		ON CONFLICT (project_id, recipient_id, batch_key, channel)
			WHERE status = 'collecting'
		DO UPDATE SET
			events = notification_batches.events || EXCLUDED.events,
			event_count = notification_batches.event_count + 1,
			window_end = GREATEST(notification_batches.window_end, EXCLUDED.window_end)
		RETURNING id, project_id, recipient_type, recipient_id, batch_key, channel, status,
		          events, event_count, window_start, window_end, sent_at, created_at`

	stored, err := scanNotificationBatch(q.db.QueryRow(ctx, query,
		batch.ID,
		batch.ProjectID,
		batch.RecipientType,
		batch.RecipientID,
		batch.BatchKey,
		batch.Channel,
		batch.Status,
		event,
		batch.WindowEnd,
	))
	if err != nil {
		return fmt.Errorf("append notification batch event: %w", err)
	}

	*batch = *stored
	return nil
}

func (q *Queries) ClaimDueNotificationBatches(ctx context.Context, limit int) ([]domain.NotificationBatch, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimDueNotificationBatches")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH due AS (
			SELECT id
			FROM notification_batches
			WHERE status = 'collecting'
			  AND window_end <= NOW()
			ORDER BY window_end ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_batches nb
		SET status = $2
		FROM due
		WHERE nb.id = due.id
		RETURNING nb.id, nb.project_id, nb.recipient_type, nb.recipient_id, nb.batch_key, nb.channel, nb.status,
		          nb.events, nb.event_count, nb.window_start, nb.window_end, nb.sent_at, nb.created_at`

	rows, err := q.db.Query(ctx, query, limit, domain.NotifyBatchStatusProcessing)
	if err != nil {
		return nil, fmt.Errorf("claim due notification batches: %w", err)
	}
	defer rows.Close()

	batches := make([]domain.NotificationBatch, 0, limit)
	for rows.Next() {
		batch, scanErr := scanNotificationBatch(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("claim due notification batches scan: %w", scanErr)
		}
		batches = append(batches, *batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim due notification batches rows: %w", err)
	}

	return batches, nil
}

func (q *Queries) MarkNotificationBatchSent(ctx context.Context, id, projectID string, sentAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkNotificationBatchSent")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE notification_batches
		 SET status = $3, sent_at = $4
		 WHERE id = $1 AND project_id = $2 AND status = $5`,
		id, projectID, domain.NotifyBatchStatusSent, sentAt, domain.NotifyBatchStatusProcessing,
	)
	if err != nil {
		return fmt.Errorf("mark notification batch sent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationBatchNotFound
	}
	return nil
}

func (q *Queries) RequeueNotificationBatch(ctx context.Context, id, projectID string, windowEnd time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RequeueNotificationBatch")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE notification_batches
		 SET status = $3, window_end = $4
		 WHERE id = $1 AND project_id = $2 AND status = $5`,
		id, projectID, domain.NotifyBatchStatusCollecting, windowEnd, domain.NotifyBatchStatusProcessing,
	)
	if err != nil {
		return fmt.Errorf("requeue notification batch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationBatchNotFound
	}
	return nil
}

func (q *Queries) MarkNotificationBatchFailed(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkNotificationBatchFailed")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE notification_batches
		 SET status = $3
		 WHERE id = $1 AND project_id = $2 AND status = $4`,
		id, projectID, domain.NotifyBatchStatusFailed, domain.NotifyBatchStatusProcessing,
	)
	if err != nil {
		return fmt.Errorf("mark notification batch failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationBatchNotFound
	}
	return nil
}

func scanNotificationBatch(scanner scanTarget) (*domain.NotificationBatch, error) {
	var batch domain.NotificationBatch
	var events []byte

	err := scanner.Scan(
		&batch.ID,
		&batch.ProjectID,
		&batch.RecipientType,
		&batch.RecipientID,
		&batch.BatchKey,
		&batch.Channel,
		&batch.Status,
		&events,
		&batch.EventCount,
		&batch.WindowStart,
		&batch.WindowEnd,
		&batch.SentAt,
		&batch.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(events) > 0 {
		batch.Events = events
	}

	return &batch, nil
}
