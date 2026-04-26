package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

func (q *Queries) GetModelRouting(ctx context.Context, agentID string) ([]domain.ModelRoute, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetModelRouting")
	defer span.End()
	rows, err := q.db.Query(ctx,
		`SELECT id, agent_id, tier, model, quality_score, previous_model, updated_at, updated_by
		 FROM agent_model_routing WHERE agent_id = $1 ORDER BY tier`, agentID)
	if err != nil {
		return nil, fmt.Errorf("get model routing: %w", err)
	}
	defer rows.Close()
	var routes []domain.ModelRoute
	for rows.Next() {
		var r domain.ModelRoute
		if err := rows.Scan(&r.ID, &r.AgentID, &r.Tier, &r.Model, &r.QualityScore, &r.PreviousModel, &r.UpdatedAt, &r.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan model route: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

func (q *Queries) GetModelRoutingByTier(ctx context.Context, agentID, tier string) (*domain.ModelRoute, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetModelRoutingByTier")
	defer span.End()
	var r domain.ModelRoute
	err := q.db.QueryRow(ctx,
		`SELECT id, agent_id, tier, model, quality_score, previous_model, updated_at, updated_by
		 FROM agent_model_routing WHERE agent_id = $1 AND tier = $2`,
		agentID, tier).Scan(&r.ID, &r.AgentID, &r.Tier, &r.Model, &r.QualityScore, &r.PreviousModel, &r.UpdatedAt, &r.UpdatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get model routing by tier: %w", err)
	}
	return &r, nil
}

func (q *Queries) UpsertModelRouting(ctx context.Context, route *domain.ModelRoute) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertModelRouting")
	defer span.End()
	query := `INSERT INTO agent_model_routing (agent_id, tier, model, quality_score, previous_model, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (agent_id, tier) DO UPDATE SET
			model = $3, quality_score = $4, previous_model = $5, updated_by = $6, updated_at = NOW()
		RETURNING id, updated_at`
	err := q.db.QueryRow(ctx, query,
		route.AgentID, route.Tier, route.Model, route.QualityScore, route.PreviousModel, route.UpdatedBy).
		Scan(&route.ID, &route.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert model routing: %w", err)
	}
	return nil
}

func (q *Queries) DeleteModelRouting(ctx context.Context, agentID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteModelRouting")
	defer span.End()
	_, err := q.db.Exec(ctx,
		`DELETE FROM agent_model_routing WHERE agent_id = $1`, agentID)
	if err != nil {
		return fmt.Errorf("delete model routing: %w", err)
	}
	return nil
}
