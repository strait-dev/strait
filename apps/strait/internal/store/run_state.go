package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertRunState(ctx context.Context, s *domain.RunState) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunState")
	defer span.End()

	query := `
		INSERT INTO run_state (run_id, state_key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (run_id, state_key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query, s.RunID, s.StateKey, s.Value).Scan(&s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert run state: %w", err)
	}
	return nil
}

func (q *Queries) GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunState")
	defer span.End()

	query := `SELECT run_id, state_key, value, updated_at FROM run_state WHERE run_id = $1 AND state_key = $2`
	var s domain.RunState
	err := q.db.QueryRow(ctx, query, runID, key).Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run state: %w", err)
	}
	return &s, nil
}

func (q *Queries) ListRunState(ctx context.Context, runID string) ([]domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunState")
	defer span.End()

	query := `SELECT run_id, state_key, value, updated_at FROM run_state WHERE run_id = $1 ORDER BY state_key ASC`
	rows, err := q.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("list run state: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RunState, 0, 16)
	for rows.Next() {
		var s domain.RunState
		if err := rows.Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list run state scan: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func (q *Queries) DeleteRunState(ctx context.Context, runID, key string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteRunState")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM run_state WHERE run_id = $1 AND state_key = $2`, runID, key)
	if err != nil {
		return fmt.Errorf("delete run state: %w", err)
	}
	return nil
}
