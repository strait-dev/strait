package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

func (q *Queries) CreateGoldenSet(ctx context.Context, gs *domain.GoldenSet) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateGoldenSet")
	defer span.End()
	query := `INSERT INTO agent_golden_sets (agent_id, project_id, name, cases)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (agent_id, name) DO UPDATE SET cases = $4, updated_at = NOW()
		RETURNING id, created_at, updated_at`
	err := q.db.QueryRow(ctx, query, gs.AgentID, gs.ProjectID, gs.Name, gs.Cases).
		Scan(&gs.ID, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create golden set: %w", err)
	}
	return nil
}

func (q *Queries) GetGoldenSet(ctx context.Context, agentID, name string) (*domain.GoldenSet, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetGoldenSet")
	defer span.End()
	var gs domain.GoldenSet
	err := q.db.QueryRow(ctx,
		`SELECT id, agent_id, project_id, name, cases, created_at, updated_at
		 FROM agent_golden_sets WHERE agent_id = $1 AND name = $2`,
		agentID, name).Scan(&gs.ID, &gs.AgentID, &gs.ProjectID, &gs.Name, &gs.Cases, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get golden set: %w", err)
	}
	return &gs, nil
}

func (q *Queries) ListGoldenSets(ctx context.Context, agentID string) ([]domain.GoldenSet, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListGoldenSets")
	defer span.End()
	rows, err := q.db.Query(ctx,
		`SELECT id, agent_id, project_id, name, cases, created_at, updated_at
		 FROM agent_golden_sets WHERE agent_id = $1 ORDER BY name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list golden sets: %w", err)
	}
	defer rows.Close()
	var sets []domain.GoldenSet
	for rows.Next() {
		var gs domain.GoldenSet
		if err := rows.Scan(&gs.ID, &gs.AgentID, &gs.ProjectID, &gs.Name, &gs.Cases, &gs.CreatedAt, &gs.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan golden set: %w", err)
		}
		sets = append(sets, gs)
	}
	return sets, rows.Err()
}

func (q *Queries) DeleteGoldenSet(ctx context.Context, agentID, name string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteGoldenSet")
	defer span.End()
	_, err := q.db.Exec(ctx,
		`DELETE FROM agent_golden_sets WHERE agent_id = $1 AND name = $2`, agentID, name)
	if err != nil {
		return fmt.Errorf("delete golden set: %w", err)
	}
	return nil
}
