package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Agent usage handler.

type AgentUsageInput struct {
	OrgID string `query:"org_id"`
}
type AgentUsageOutput struct {
	Body AgentUsageResponse
}
type AgentUsageResponse struct {
	AgentPlanTier     string  `json:"agent_plan_tier"`
	IncludedCreditUsd float64 `json:"included_credit_usd"`
	UsedCreditUsd     float64 `json:"used_credit_usd"`
	OverageUsd        float64 `json:"overage_usd"`
	RunCount          int64   `json:"run_count"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalToolCalls    int64   `json:"total_tool_calls"`
	TotalCostMicrousd int64   `json:"total_cost_microusd"`

	UpgradeRecommended bool   `json:"upgrade_recommended"`
	UpgradeReason      string `json:"upgrade_reason,omitempty"`
}

func (s *Server) handleGetAgentUsage(ctx context.Context, input *AgentUsageInput) (*AgentUsageOutput, error) {
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}

	agentTier := domain.AgentPlanFree
	agentLimits := billing.GetAgentPlanLimits(agentTier)

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	var totalCost, totalTokens, totalToolCalls, runCount int64
	rows, qErr := s.store.QueryAgentUsageSummary(ctx, orgID, periodStart)
	if qErr != nil {
		slog.Error("failed to query agent usage summary", "org_id", orgID, "error", qErr)
	} else if rows != nil {
		totalCost = rows.TotalCostMicrousd
		totalTokens = rows.TotalTokens
		totalToolCalls = rows.TotalToolCalls
		runCount = rows.RunCount
	}

	includedCredit := float64(agentLimits.AgentCreditMicrousd) / 1e6
	usedCredit := float64(totalCost) / 1e6
	overage := 0.0
	if usedCredit > includedCredit {
		overage = usedCredit - includedCredit
	}

	upgradeRecommended := false
	upgradeReason := ""
	if agentTier == domain.AgentPlanMaker && overage >= 100 {
		upgradeRecommended = true
		upgradeReason = fmt.Sprintf("Your agent overage of $%.0f/mo exceeds $100. Growth plan ($149/mo) includes $149 in credits with full orchestration.", overage)
	}

	return &AgentUsageOutput{Body: AgentUsageResponse{
		AgentPlanTier:      string(agentTier),
		IncludedCreditUsd:  includedCredit,
		UsedCreditUsd:      usedCredit,
		OverageUsd:         overage,
		RunCount:           runCount,
		TotalTokens:        totalTokens,
		TotalToolCalls:     totalToolCalls,
		TotalCostMicrousd:  totalCost,
		UpgradeRecommended: upgradeRecommended,
		UpgradeReason:      upgradeReason,
	}}, nil
}

// Agent spending limit handlers.

type AgentSpendingLimitInput struct {
	OrgID string `query:"org_id"`
}
type AgentSpendingLimitOutput struct {
	Body AgentSpendingLimitResponse
}
type AgentSpendingLimitResponse struct {
	LimitMicrousd int64   `json:"limit_microusd"` // -1 = no limit.
	LimitUsd      float64 `json:"limit_usd"`
	Enabled       bool    `json:"enabled"`
}

func (s *Server) handleGetAgentSpendingLimit(ctx context.Context, input *AgentSpendingLimitInput) (*AgentSpendingLimitOutput, error) {
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}

	limit, storeErr := s.store.GetOrgAgentSpendingLimit(ctx, orgID)
	if storeErr != nil {
		slog.Warn("failed to get agent spending limit", "org_id", orgID, "error", storeErr)
		limit = -1
	}

	limitUsd := float64(limit) / 1e6
	if limit < 0 {
		limitUsd = -1
	}
	return &AgentSpendingLimitOutput{Body: AgentSpendingLimitResponse{
		LimitMicrousd: limit,
		LimitUsd:      limitUsd,
		Enabled:       limit > 0,
	}}, nil
}

type UpdateAgentSpendingLimitInput struct {
	OrgID string `query:"org_id"`
	Body  UpdateAgentSpendingLimitRequest
}
type UpdateAgentSpendingLimitRequest struct {
	LimitMicrousd int64 `json:"limit_microusd"` // -1 to disable.
}
type UpdateAgentSpendingLimitOutput struct {
	Body AgentSpendingLimitResponse
}

func (s *Server) handleUpdateAgentSpendingLimit(ctx context.Context, input *UpdateAgentSpendingLimitInput) (*UpdateAgentSpendingLimitOutput, error) {
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}

	limit := input.Body.LimitMicrousd
	if limit < -1 {
		return nil, huma.Error400BadRequest("limit_microusd must be -1 (disabled) or a positive value")
	}

	if storeErr := s.store.UpdateAgentSpendingLimit(ctx, orgID, limit); storeErr != nil {
		slog.Error("failed to update agent spending limit", "org_id", orgID, "error", storeErr)
		return nil, huma.Error500InternalServerError("failed to update spending limit")
	}

	limitUsd := float64(limit) / 1e6
	if limit < 0 {
		limitUsd = -1
	}
	return &UpdateAgentSpendingLimitOutput{Body: AgentSpendingLimitResponse{
		LimitMicrousd: limit,
		LimitUsd:      limitUsd,
		Enabled:       limit > 0,
	}}, nil
}
