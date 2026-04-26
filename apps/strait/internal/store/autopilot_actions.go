package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

func (q *Queries) CreateAutopilotAction(ctx context.Context, action *domain.AutopilotAction) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAutopilotAction")
	defer span.End()

	query := `INSERT INTO agent_autopilot_actions (agent_id, tier, previous_model, new_model, budget_pct, quality_score, action, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`
	err := q.db.QueryRow(ctx, query,
		action.AgentID, action.Tier, action.PreviousModel, action.NewModel,
		action.BudgetPct, action.QualityScore, action.Action, action.Reason).
		Scan(&action.ID, &action.CreatedAt)
	if err != nil {
		return fmt.Errorf("create autopilot action: %w", err)
	}
	return nil
}

func (q *Queries) ListAutopilotActions(ctx context.Context, agentID string, limit int) ([]domain.AutopilotAction, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAutopilotActions")
	defer span.End()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := q.db.Query(ctx,
		`SELECT id, agent_id, tier, previous_model, new_model, budget_pct, quality_score, action, reason, created_at
		 FROM agent_autopilot_actions WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2`,
		agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list autopilot actions: %w", err)
	}
	defer rows.Close()

	var actions []domain.AutopilotAction
	for rows.Next() {
		var a domain.AutopilotAction
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Tier, &a.PreviousModel, &a.NewModel,
			&a.BudgetPct, &a.QualityScore, &a.Action, &a.Reason, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan autopilot action: %w", err)
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

func (q *Queries) GetLatestAutopilotAction(ctx context.Context, agentID string) (*domain.AutopilotAction, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestAutopilotAction")
	defer span.End()

	var a domain.AutopilotAction
	err := q.db.QueryRow(ctx,
		`SELECT id, agent_id, tier, previous_model, new_model, budget_pct, quality_score, action, reason, created_at
		 FROM agent_autopilot_actions WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 1`,
		agentID).Scan(&a.ID, &a.AgentID, &a.Tier, &a.PreviousModel, &a.NewModel,
		&a.BudgetPct, &a.QualityScore, &a.Action, &a.Reason, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest autopilot action: %w", err)
	}
	return &a, nil
}
