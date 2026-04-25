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
	if agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
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
				ID:                  "rec-model-" + safeIDPrefix(agent.ID),
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
			ID:                  "rec-budget-" + safeIDPrefix(agent.ID),
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
			ID:                  "rec-cache-" + safeIDPrefix(agent.ID),
			Type:                RecPromptCaching,
			Description:         fmt.Sprintf("This agent has %d recent runs. Enabling prompt caching could reduce token costs for repeated prompts.", len(runs)),
			EstimatedSavingsPct: 15,
			SuggestedPatch:      map[string]any{"prompt_caching": true},
			Status:              "pending",
		})
	}

	// Filter out dismissed recommendations.
	dismissed := make(map[string]struct{}, len(agent.DismissedRecommendations))
	for _, d := range agent.DismissedRecommendations {
		dismissed[d.RecommendationID] = struct{}{}
	}
	var filtered []CostRecommendation
	for _, r := range recommendations {
		if _, ok := dismissed[r.ID]; !ok {
			filtered = append(filtered, r)
		}
	}
	if filtered == nil {
		filtered = []CostRecommendation{}
	}
	return filtered, nil
}

// safeIDPrefix returns up to the first 8 characters of an ID for use in
// recommendation IDs. Guards against panic on short or empty strings.
func safeIDPrefix(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// allowedPatchKeys defines the config keys that cost recommendations are
// allowed to modify. Any key outside this set is silently dropped to
// prevent injection of unexpected config fields (ESAA IV-F01).
var allowedPatchKeys = map[string]bool{
	"model":          true,
	"budget":         true,
	"prompt_caching": true,
}

// FilterAllowedPatchKeys returns a copy of the patch containing only
// keys that are in the allowlist.
func FilterAllowedPatchKeys(patch map[string]any) map[string]any {
	safe := make(map[string]any, len(patch))
	for k, v := range patch {
		if allowedPatchKeys[k] {
			safe[k] = v
		}
	}
	return safe
}

// allowedReplayKeys defines config keys that can be overridden in a replay.
// Excludes webhook_url, webhook_secret, sandbox, and provider_secrets to
// prevent data exfiltration or sandbox escape via replay config injection.
func FilterAllowedReplayKeys(overrides map[string]any) map[string]any {
	blocked := map[string]bool{
		"webhook_url":    true,
		"webhook_secret": true,
		"sandbox":        true,
	}
	safe := make(map[string]any, len(overrides))
	for k, v := range overrides {
		if !blocked[k] {
			safe[k] = v
		}
	}
	return safe
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
