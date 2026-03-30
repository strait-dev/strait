package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"
)

// CostRecommendationType identifies the kind of cost optimization recommendation.
type CostRecommendationType string

const (
	RecModelDowngrade  CostRecommendationType = "model_downgrade"
	RecBudgetReduction CostRecommendationType = "budget_reduction"
	RecPromptCaching   CostRecommendationType = "prompt_caching"
)

// CostRecommendation represents a single cost optimization suggestion.
type CostRecommendation struct {
	ID                  string                 `json:"id"`
	Type                CostRecommendationType `json:"type"`
	Description         string                 `json:"description"`
	EstimatedSavingsPct float64                `json:"estimated_savings_pct"`
	SuggestedPatch      map[string]any         `json:"suggested_patch"`
	Status              string                 `json:"status"` // "pending", "applied", "dismissed"
}

// CostOptimizerStore defines the store methods needed by the optimizer.
type CostOptimizerStore interface {
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
}

// GenerateRecommendations analyzes an agent's recent runs and returns
// cost optimization recommendations.
func GenerateRecommendations(ctx context.Context, store CostOptimizerStore, agent *domain.Agent) ([]CostRecommendation, error) {
	runs, err := store.ListRunsByJob(ctx, agent.JobID, 50, 0)
	if err != nil {
		return nil, fmt.Errorf("list agent runs for optimization: %w", err)
	}

	if len(runs) < 5 {
		return nil, nil // Not enough data
	}

	var recommendations []CostRecommendation

	// Count terminal statuses.
	var completed, failed int
	for _, run := range runs {
		switch run.Status {
		case domain.StatusCompleted:
			completed++
		case domain.StatusFailed, domain.StatusSystemFailed:
			failed++
		default:
			// skip non-terminal or other terminal statuses
		}
	}
	total := completed + failed
	if total == 0 {
		return nil, nil
	}

	successRate := float64(completed) / float64(total)

	// Recommendation 1: Model downgrade if success rate is high.
	if successRate > 0.95 && isExpensiveModel(agent.Model) {
		cheaper := suggestCheaperModel(agent.Model)
		if cheaper != "" {
			recommendations = append(recommendations, CostRecommendation{
				ID:                  "rec-model-" + agent.ID[:8],
				Type:                RecModelDowngrade,
				Description:         fmt.Sprintf("Success rate is %.0f%%. Consider switching from %s to %s for lower cost.", successRate*100, agent.Model, cheaper),
				EstimatedSavingsPct: 40,
				SuggestedPatch:      map[string]any{"model": cheaper},
				Status:              "pending",
			})
		}
	}

	// Recommendation 2: Budget reduction if actual spend is low.
	var budgetLimit int
	if len(agent.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(agent.Config, &cfg); err == nil {
			if b, ok := cfg["budget"].(string); ok && b != "" {
				budgetLimit = 1 // has budget
			}
		}
	}
	if budgetLimit == 0 {
		recommendations = append(recommendations, CostRecommendation{
			ID:                  "rec-budget-" + agent.ID[:8],
			Type:                RecBudgetReduction,
			Description:         "No budget limit configured. Adding a budget protects against runaway costs.",
			EstimatedSavingsPct: 0,
			SuggestedPatch:      map[string]any{"budget": "$5.00"},
			Status:              "pending",
		})
	}

	// Recommendation 3: Prompt caching.
	if len(runs) > 20 {
		recommendations = append(recommendations, CostRecommendation{
			ID:                  "rec-cache-" + agent.ID[:8],
			Type:                RecPromptCaching,
			Description:         fmt.Sprintf("This agent has %d recent runs. Enabling prompt caching could reduce token costs for repeated prompts.", len(runs)),
			EstimatedSavingsPct: 15,
			SuggestedPatch:      map[string]any{"prompt_caching": true},
			Status:              "pending",
		})
	}

	return recommendations, nil
}

func isExpensiveModel(model string) bool {
	switch model {
	case "gpt-5.4", "claude-opus-4-6", "claude-sonnet-4-6":
		return true
	default:
		return false
	}
}

func suggestCheaperModel(model string) string {
	switch model {
	case "gpt-5.4":
		return "gpt-5.4-mini"
	case "claude-opus-4-6":
		return "claude-sonnet-4-6"
	case "claude-sonnet-4-6":
		return "claude-haiku-4-5"
	default:
		return ""
	}
}
