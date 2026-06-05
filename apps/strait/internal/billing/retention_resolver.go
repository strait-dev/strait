package billing

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"
)

// PlanRetentionResolver implements the scheduler.OrgRetentionResolver interface
// by looking up each org's plan tier and returning the corresponding retention days.
// It accounts for active table-backed history add-ons when present.
type PlanRetentionResolver struct {
	store Store
}

// NewPlanRetentionResolver creates a resolver that reads retention days from plan limits.
func NewPlanRetentionResolver(store Store) *PlanRetentionResolver {
	return &PlanRetentionResolver{store: store}
}

// ListAllSubscribedOrgIDs returns all org IDs with active subscriptions.
func (r *PlanRetentionResolver) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	return r.store.ListAllSubscribedOrgIDs(ctx)
}

// GetOrgRetentionDays returns the retention period for an org based on their
// plan tier plus active table-backed history add-ons.
func (r *PlanRetentionResolver) GetOrgRetentionDays(ctx context.Context, orgID string) (int, error) {
	if orgID == "" {
		return 0, errors.New("org id is required")
	}

	sub, err := r.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return 0, ErrSubscriptionNotFound
		}
		return 0, fmt.Errorf("get org subscription for retention: %w", err)
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	addons, err := r.store.ListActiveAddons(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("list active add-ons for retention: %w", err)
	}
	limits = EffectiveLimits(limits, addons)
	limits = ApplySubscriptionAddOns(limits, sub.AddOns)
	return limits.RetentionDays, nil
}
