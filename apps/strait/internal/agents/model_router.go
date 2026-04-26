package agents

import (
	"context"
	"fmt"

	"strait/internal/domain"
)

// RequestTier classifies agent requests by complexity.
type RequestTier string

const (
	TierSimple   RequestTier = "simple"
	TierStandard RequestTier = "standard"
	TierComplex  RequestTier = "complex"
)

const defaultQualityThreshold = 85.0

// ClassifyRequest determines the tier based on prompt size and tool usage.
func ClassifyRequest(promptTokenEstimate int, toolCount int, hasStructuredOutput bool) RequestTier {
	if hasStructuredOutput || promptTokenEstimate > 4000 || toolCount >= 4 {
		return TierComplex
	}
	if promptTokenEstimate >= 500 || toolCount > 0 {
		return TierStandard
	}
	return TierSimple
}

// modelRoutingStore defines the store methods used by the model router.
type modelRoutingStore interface {
	GetModelRouting(ctx context.Context, agentID string) ([]domain.ModelRoute, error)
	GetModelRoutingByTier(ctx context.Context, agentID, tier string) (*domain.ModelRoute, error)
	UpsertModelRouting(ctx context.Context, route *domain.ModelRoute) error
}

// ModelRouter resolves which model to use for a given request tier.
type ModelRouter struct {
	store modelRoutingStore
}

// NewModelRouter creates a new model router.
func NewModelRouter(store modelRoutingStore) *ModelRouter {
	return &ModelRouter{store: store}
}

// ResolveModel returns the model for a given tier, falling back to defaultModel.
func (r *ModelRouter) ResolveModel(ctx context.Context, agentID, defaultModel string, tier RequestTier) (string, error) {
	route, err := r.store.GetModelRoutingByTier(ctx, agentID, string(tier))
	if err != nil {
		return defaultModel, fmt.Errorf("resolve model: %w", err)
	}
	if route == nil || route.Model == "" {
		return defaultModel, nil
	}
	return route.Model, nil
}

// CheckQualityGate compares the latest eval score against the threshold.
// If below threshold and a previous model exists, reverts to it.
func (r *ModelRouter) CheckQualityGate(ctx context.Context, agentID string, tier RequestTier, latestScore float64) error {
	route, err := r.store.GetModelRoutingByTier(ctx, agentID, string(tier))
	if err != nil {
		return fmt.Errorf("check quality gate: %w", err)
	}
	if route == nil {
		return nil
	}

	if latestScore < defaultQualityThreshold && route.PreviousModel != "" {
		// Revert to previous model.
		route.Model, route.PreviousModel = route.PreviousModel, route.Model
		route.QualityScore = latestScore
		return r.store.UpsertModelRouting(ctx, route)
	}

	// Update quality score.
	route.QualityScore = latestScore
	return r.store.UpsertModelRouting(ctx, route)
}
