package billing

import (
	"context"
	"errors"

	"strait/internal/domain"
)

// PlanRetentionResolver implements the scheduler.OrgRetentionResolver interface
// by looking up each org's plan tier and returning the corresponding retention days.
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

// GetOrgRetentionDays returns the retention period for an org based on their plan tier.
// Falls back to free-tier retention (1 day) on any error.
func (r *PlanRetentionResolver) GetOrgRetentionDays(ctx context.Context, orgID string) (int, error) {
	if orgID == "" {
		return GetPlanLimits(domain.PlanFree).RetentionDays, nil
	}

	sub, err := r.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return GetPlanLimits(domain.PlanFree).RetentionDays, nil
		}
		return GetPlanLimits(domain.PlanFree).RetentionDays, nil
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	return limits.RetentionDays, nil
}
