package store

import (
	"context"
	"encoding/json"
	"fmt"

	"orchestrator/internal/domain"
	"orchestrator/internal/dbscan"

	"github.com/google/uuid"
)

func (q *Queries) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
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

func (q *Queries) ListEvents(ctx context.Context, runID string) ([]domain.RunEvent, error) {
	query := `
		SELECT id, run_id, type, level, message, data, created_at
		FROM run_events
		WHERE run_id = $1
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.RunEvent, 0)
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
