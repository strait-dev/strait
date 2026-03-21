package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.InsertEvent")
	defer span.End()

	if event.ID == "" {
		event.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_events (id, run_id, type, level, message, data)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		event.ID,
		event.RunID,
		event.Type,
		dbscan.NilIfEmptyString(event.Level),
		event.Message,
		dbscan.NilIfEmptyRawMessage(event.Data),
	).Scan(&event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

func (q *Queries) ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEvents")
	defer span.End()

	query := `
		SELECT id, run_id, type, level, message, data, created_at
		FROM run_events
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.RunEvent, 0, 16)
	for rows.Next() {
		var event domain.RunEvent
		var level *string
		var data []byte

		err := rows.Scan(
			&event.ID,
			&event.RunID,
			&event.Type,
			&level,
			&event.Message,
			&data,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("list events scan: %w", err)
		}

		if level != nil {
			event.Level = *level
		}
		if data != nil {
			event.Data = json.RawMessage(data)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events rows: %w", err)
	}

	return events, nil
}

func (q *Queries) ListEventsAsc(ctx context.Context, runID string, limit int, afterTime *time.Time, afterID string) ([]domain.RunEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventsAsc")
	defer span.End()

	query := `
		SELECT id, run_id, type, level, message, data, created_at
		FROM run_events
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if afterTime != nil {
		query += fmt.Sprintf(" AND (created_at > $%d OR (created_at = $%d AND id > $%d))", param, param, param+1)
		args = append(args, *afterTime, afterID)
		param += 2
	}

	query += fmt.Sprintf(" ORDER BY created_at ASC, id ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events asc: %w", err)
	}
	defer rows.Close()

	events := make([]domain.RunEvent, 0, 16)
	for rows.Next() {
		var event domain.RunEvent
		var level *string
		var data []byte

		err := rows.Scan(
			&event.ID,
			&event.RunID,
			&event.Type,
			&level,
			&event.Message,
			&data,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("list events asc scan: %w", err)
		}

		if level != nil {
			event.Level = *level
		}
		if data != nil {
			event.Data = json.RawMessage(data)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events asc rows: %w", err)
	}

	return events, nil
}

func (q *Queries) ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventsByRunFiltered")
	defer span.End()

	baseQuery := `
		SELECT id, run_id, type, level, message, data, created_at
		FROM run_events
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if level != "" {
		baseQuery += fmt.Sprintf(" AND level = $%d", param)
		args = append(args, level)
		param++
	}

	if eventType != "" {
		baseQuery += fmt.Sprintf(" AND type = $%d", param)
		args = append(args, eventType)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list events filtered: %w", err)
	}
	defer rows.Close()

	events := make([]domain.RunEvent, 0, 16)
	for rows.Next() {
		var event domain.RunEvent
		var lvl *string
		var data []byte

		err := rows.Scan(
			&event.ID, &event.RunID, &event.Type, &lvl,
			&event.Message, &data, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("list events filtered scan: %w", err)
		}
		if lvl != nil {
			event.Level = *lvl
		}
		if data != nil {
			event.Data = json.RawMessage(data)
		}
		events = append(events, event)
	}

	return events, rows.Err()
}
