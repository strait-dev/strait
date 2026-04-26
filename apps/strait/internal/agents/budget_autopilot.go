package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
)

// AutopilotStore defines store methods for budget autopilot.
type AutopilotStore interface {
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
	CreateAutopilotAction(ctx context.Context, action *domain.AutopilotAction) error
	GetLatestAutopilotAction(ctx context.Context, agentID string) (*domain.AutopilotAction, error)
}

// BudgetAutopilot monitors agent spending and auto-downgrades model tiers.
type BudgetAutopilot struct {
	store       AutopilotStore
	modelRouter *ModelRouter
}

// NewBudgetAutopilot creates a new budget autopilot.
func NewBudgetAutopilot(store AutopilotStore, router *ModelRouter) *BudgetAutopilot {
	return &BudgetAutopilot{store: store, modelRouter: router}
}

// CheckAndAdjust evaluates an agent's spending against its budget and downgrades if needed.
func (b *BudgetAutopilot) CheckAndAdjust(ctx context.Context, agentID string, currentSpendMicrousd int64) (*domain.AutopilotAction, error) {
	agent, err := b.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	// Parse autopilot config from agent.Config.
	var cfg domain.AutopilotConfig
	if len(agent.Config) > 0 {
		var configMap map[string]json.RawMessage
		if err := json.Unmarshal(agent.Config, &configMap); err == nil {
			if raw, ok := configMap["autopilot"]; ok {
				_ = json.Unmarshal(raw, &cfg)
			}
		}
	}

	if !cfg.Enabled || cfg.BudgetMicrousd <= 0 {
		return nil, nil // autopilot not configured
	}

	if cfg.QualityThreshold <= 0 {
		cfg.QualityThreshold = 85.0
	}
	if cfg.ObservationMins <= 0 {
		cfg.ObservationMins = 10
	}

	// Check observation window.
	lastAction, _ := b.store.GetLatestAutopilotAction(ctx, agentID)
	if lastAction != nil {
		elapsed := time.Since(lastAction.CreatedAt)
		remainingPct := 1.0 - float64(currentSpendMicrousd)/float64(cfg.BudgetMicrousd)
		// Skip observation window if remaining budget < 5%.
		if remainingPct >= 0.05 && elapsed < time.Duration(cfg.ObservationMins)*time.Minute {
			return nil, nil // within observation window
		}
	}

	budgetPct := float64(currentSpendMicrousd) / float64(cfg.BudgetMicrousd) * 100

	// Determine which tier to downgrade.
	var targetTier RequestTier
	switch {
	case budgetPct >= 90:
		targetTier = TierStandard
	case budgetPct >= 80:
		targetTier = TierSimple
	default:
		return nil, nil // under threshold
	}

	cheapestModel := cfg.CheapestModel
	if cheapestModel == "" {
		cheapestModel = "gpt-4o-mini" // default
	}

	// Resolve current model for the tier.
	currentModel, err := b.modelRouter.ResolveModel(ctx, agentID, agent.Model, targetTier)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}

	if currentModel == cheapestModel {
		// Already at cheapest model for this tier.
		return nil, nil
	}

	// Apply downgrade via model router.
	route := &domain.ModelRoute{
		AgentID:       agentID,
		Tier:          string(targetTier),
		Model:         cheapestModel,
		PreviousModel: currentModel,
		UpdatedBy:     "autopilot",
	}
	if err := b.modelRouter.SetModelForTier(ctx, route); err != nil {
		return nil, fmt.Errorf("upsert model routing: %w", err)
	}

	action := &domain.AutopilotAction{
		AgentID:       agentID,
		Tier:          string(targetTier),
		PreviousModel: currentModel,
		NewModel:      cheapestModel,
		BudgetPct:     budgetPct,
		Action:        "downgrade",
		Reason:        fmt.Sprintf("budget %.1f%% consumed, downgrading %s tier to %s", budgetPct, targetTier, cheapestModel),
	}
	if err := b.store.CreateAutopilotAction(ctx, action); err != nil {
		slog.Error("create autopilot action", "agent_id", agentID, "error", err)
	}

	return action, nil
}
