package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// CreateAgentUsageRecord inserts a billing usage record for an agent run.
// The run_id unique index prevents double-billing on retries.
func (q *Queries) CreateAgentUsageRecord(ctx context.Context, rec *domain.AgentUsageRecord) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAgentUsageRecord")
	defer span.End()

	if rec.ID == "" {
		rec.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO agent_usage_records (
			id, run_id, project_id, org_id, agent_id,
			total_tokens, tool_call_count,
			run_cost_microusd, token_cost_microusd, tool_cost_microusd, total_cost_microusd
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (run_id) DO NOTHING
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		rec.ID, rec.RunID, rec.ProjectID, rec.OrgID, rec.AgentID,
		rec.TotalTokens, rec.ToolCallCount,
		rec.RunCostMicrousd, rec.TokenCostMicrousd, rec.ToolCostMicrousd, rec.TotalCostMicrousd,
	).Scan(&rec.CreatedAt)
	if err != nil {
		return fmt.Errorf("create agent usage record: %w", err)
	}

	return nil
}

// AgentUsageSummary is an aggregate of agent usage for a billing period.
type AgentUsageSummary struct {
	RunCount          int64
	TotalTokens       int64
	TotalToolCalls    int64
	TotalCostMicrousd int64
}

// QueryAgentUsageSummary returns aggregated agent usage for an org since the given time.
func (q *Queries) QueryAgentUsageSummary(ctx context.Context, orgID string, since time.Time) (*AgentUsageSummary, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.QueryAgentUsageSummary")
	defer span.End()

	var s AgentUsageSummary
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(tool_call_count), 0),
			COALESCE(SUM(total_cost_microusd), 0)
		FROM agent_usage_records
		WHERE org_id = $1 AND created_at >= $2
	`, orgID, since).Scan(&s.RunCount, &s.TotalTokens, &s.TotalToolCalls, &s.TotalCostMicrousd)
	if err != nil {
		return nil, fmt.Errorf("query agent usage summary: %w", err)
	}
	return &s, nil
}

// GetOrgAgentSpendingLimit returns the agent spending limit for an org.
// Returns -1 if no limit is set.
func (q *Queries) GetOrgAgentSpendingLimit(ctx context.Context, orgID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetOrgAgentSpendingLimit")
	defer span.End()

	var limit int64
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(agent_spending_limit_microusd, -1)
		FROM organization_subscriptions
		WHERE org_id = $1
	`, orgID).Scan(&limit)
	if err != nil {
		return -1, fmt.Errorf("get org agent spending limit: %w", err)
	}
	return limit, nil
}

// UpdateAgentSpendingLimit sets the agent spending limit for an org.
// Pass -1 to disable the limit.
func (q *Queries) UpdateAgentSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateAgentSpendingLimit")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		UPDATE organization_subscriptions
		SET agent_spending_limit_microusd = $2, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, limitMicrousd)
	if err != nil {
		return fmt.Errorf("update agent spending limit: %w", err)
	}
	return nil
}

// SumOrgAgentSpendSince returns the total agent billing cost (micro-USD) for an
// org since the given timestamp. Used for spending limit enforcement.
func (q *Queries) SumOrgAgentSpendSince(ctx context.Context, orgID string, since time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumOrgAgentSpendSince")
	defer span.End()

	query := `
		SELECT COALESCE(SUM(total_cost_microusd), 0)
		FROM agent_usage_records
		WHERE org_id = $1 AND created_at >= $2`

	var total int64
	err := q.db.QueryRow(ctx, query, orgID, since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum org agent spend: %w", err)
	}

	return total, nil
}
