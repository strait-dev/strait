package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotifySuppressionEvent(ctx context.Context, event *domain.NotifySuppressionEvent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotifySuppressionEvent")
	defer span.End()

	if event.ID == "" {
		event.ID = uuid.Must(uuid.NewV7()).String()
	}
	if event.Scope == "" {
		event.Scope = "global"
	}
	if len(event.Metadata) == 0 {
		event.Metadata = []byte("{}")
	}

	const query = `
		INSERT INTO notify_suppression_events (
			id,
			project_id,
			recipient_type,
			recipient_id,
			scope,
			channel,
			action,
			reason,
			source,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		event.ID,
		event.ProjectID,
		event.RecipientType,
		event.RecipientID,
		event.Scope,
		event.Channel,
		event.Action,
		dbscan.NilIfEmptyString(event.Reason),
		event.Source,
		dbscan.NilIfEmptyRawMessage(event.Metadata),
	).Scan(&event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notify suppression event: %w", err)
	}

	return nil
}

func (q *Queries) ListNotifySuppressionEvents(ctx context.Context, projectID, recipientType, recipientID string, limit int, cursor *time.Time) ([]domain.NotifySuppressionEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifySuppressionEvents")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, recipient_type, recipient_id, scope, channel, action, reason, source, metadata, created_at
		FROM notify_suppression_events
		WHERE project_id = $1
		  AND recipient_type = $2
		  AND recipient_id = $3
		  AND ($4::timestamptz IS NULL OR created_at < $4)
		ORDER BY created_at DESC
		LIMIT $5`

	var cursorValue any
	if cursor != nil {
		cursorValue = *cursor
	}

	rows, err := q.db.Query(ctx, query, projectID, recipientType, recipientID, cursorValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list notify suppression events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.NotifySuppressionEvent, 0, limit)
	for rows.Next() {
		event, scanErr := scanNotifySuppressionEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify suppression events scan: %w", scanErr)
		}
		events = append(events, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify suppression events rows: %w", err)
	}

	return events, nil
}

func scanNotifySuppressionEvent(scanner scanTarget) (*domain.NotifySuppressionEvent, error) {
	var event domain.NotifySuppressionEvent
	var reason *string
	var metadata []byte

	err := scanner.Scan(
		&event.ID,
		&event.ProjectID,
		&event.RecipientType,
		&event.RecipientID,
		&event.Scope,
		&event.Channel,
		&event.Action,
		&reason,
		&event.Source,
		&metadata,
		&event.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if reason != nil {
		event.Reason = *reason
	}
	if len(metadata) > 0 {
		event.Metadata = metadata
	}

	return &event, nil
}
