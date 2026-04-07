package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (q *Queries) GetLatestNotifySuppressionEvent(ctx context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestNotifySuppressionEvent")
	defer span.End()

	if scope == "" {
		scope = "global"
	}

	const query = `
		SELECT id, project_id, recipient_type, recipient_id, scope, channel, action, reason, source, metadata, created_at
		FROM notify_suppression_events
		WHERE project_id = $1
		  AND recipient_type = $2
		  AND recipient_id = $3
		  AND scope = $4
		  AND channel = $5
		ORDER BY created_at DESC
		LIMIT 1`

	event, err := scanNotifySuppressionEvent(q.db.QueryRow(ctx, query, projectID, recipientType, recipientID, scope, channel))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotifySuppressionEventNotFound
		}
		return nil, fmt.Errorf("get latest notify suppression event: %w", err)
	}

	return event, nil
}

func (q *Queries) DeleteOldNotifySuppressionEvents(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteOldNotifySuppressionEvents")
	defer span.End()

	if before.IsZero() {
		before = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	if limit <= 0 {
		limit = 1000
	}

	tag, err := q.db.Exec(ctx, `
		DELETE FROM notify_suppression_events
		WHERE ctid IN (
			SELECT ctid
			FROM notify_suppression_events
			WHERE created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		)
	`, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete old notify suppression events: %w", err)
	}

	return tag.RowsAffected(), nil
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
