package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

var (
	ErrEventSourceNotFound       = errors.New("event source not found")
	ErrEventSubscriptionNotFound = errors.New("event subscription not found")
)

func (q *Queries) CreateEventSource(ctx context.Context, src *domain.EventSource) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEventSource")
	defer span.End()

	if src.ID == "" {
		src.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO event_sources (id, project_id, name, description, schema, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`, src.ID, src.ProjectID, src.Name, src.Description, src.Schema, src.Enabled,
	).Scan(&src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create event source: %w", err)
	}
	return nil
}

func (q *Queries) GetEventSource(ctx context.Context, sourceID, projectID string) (*domain.EventSource, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventSource")
	defer span.End()

	var src domain.EventSource
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, name, description, schema, enabled, created_at, updated_at
		FROM event_sources WHERE id = $1 AND project_id = $2
	`, sourceID, projectID).Scan(
		&src.ID, &src.ProjectID, &src.Name, &src.Description, &src.Schema,
		&src.Enabled, &src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventSourceNotFound
		}
		return nil, fmt.Errorf("get event source: %w", err)
	}
	return &src, nil
}

func (q *Queries) GetEventSourceByName(ctx context.Context, projectID, name string) (*domain.EventSource, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventSourceByName")
	defer span.End()

	var src domain.EventSource
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, name, description, schema, enabled, created_at, updated_at
		FROM event_sources WHERE project_id = $1 AND name = $2
	`, projectID, name).Scan(
		&src.ID, &src.ProjectID, &src.Name, &src.Description, &src.Schema,
		&src.Enabled, &src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventSourceNotFound
		}
		return nil, fmt.Errorf("get event source by name: %w", err)
	}
	return &src, nil
}

func (q *Queries) ListEventSources(ctx context.Context, projectID string) ([]domain.EventSource, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventSources")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, name, description, schema, enabled, created_at, updated_at
		FROM event_sources WHERE project_id = $1 ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list event sources: %w", err)
	}
	defer rows.Close()

	var sources []domain.EventSource
	for rows.Next() {
		var src domain.EventSource
		if err := rows.Scan(&src.ID, &src.ProjectID, &src.Name, &src.Description, &src.Schema,
			&src.Enabled, &src.CreatedAt, &src.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list event sources scan: %w", err)
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

func (q *Queries) UpdateEventSource(ctx context.Context, sourceID, projectID string, patch map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateEventSource")
	defer span.End()

	patch["updated_at"] = time.Now()
	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, 2+len(patch))
	args = append(args, sourceID, projectID)
	param := 3
	for k, v := range patch {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, param))
		args = append(args, v)
		param++
	}

	query := fmt.Sprintf("UPDATE event_sources SET %s WHERE id = $1 AND project_id = $2",
		strings.Join(setClauses, ", "))
	result, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update event source: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrEventSourceNotFound
	}
	return nil
}

func (q *Queries) DeleteEventSource(ctx context.Context, sourceID, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteEventSource")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		DELETE FROM event_sources WHERE id = $1 AND project_id = $2
	`, sourceID, projectID)
	if err != nil {
		return fmt.Errorf("delete event source: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrEventSourceNotFound
	}
	return nil
}

func (q *Queries) CreateEventSubscription(ctx context.Context, sub *domain.EventSubscription) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEventSubscription")
	defer span.End()

	if sub.ID == "" {
		sub.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO event_subscriptions (id, source_id, target_type, target_id, filter_expr, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`, sub.ID, sub.SourceID, sub.TargetType, sub.TargetID, sub.FilterExpr, sub.Enabled,
	).Scan(&sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("create event subscription: %w", err)
	}
	return nil
}

func (q *Queries) ListEventSubscriptionsBySource(ctx context.Context, sourceID string) ([]domain.EventSubscription, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventSubscriptionsBySource")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, source_id, target_type, target_id, filter_expr, enabled, created_at
		FROM event_subscriptions WHERE source_id = $1 ORDER BY created_at DESC
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list event subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []domain.EventSubscription
	for rows.Next() {
		var sub domain.EventSubscription
		if err := rows.Scan(&sub.ID, &sub.SourceID, &sub.TargetType, &sub.TargetID,
			&sub.FilterExpr, &sub.Enabled, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("list event subscriptions scan: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (q *Queries) DeleteEventSubscription(ctx context.Context, subID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteEventSubscription")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		DELETE FROM event_subscriptions WHERE id = $1
	`, subID)
	if err != nil {
		return fmt.Errorf("delete event subscription: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrEventSubscriptionNotFound
	}
	return nil
}
