package billing

import (
	"context"
	"errors"

	"strait/internal/domain"
)

// retentionPackDays is the number of extra retention days granted per retention_pack unit.
const retentionPackDays = 30

// PlanRetentionResolver implements the scheduler.OrgRetentionResolver interface
// by looking up each org's plan tier and returning the corresponding retention days.
// It accounts for add-on retention packs when present.
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

// GetOrgRetentionDays returns the retention period for an org based on their plan tier
// plus any purchased retention packs from add_ons.
// Falls back to free-tier retention on any error.
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
	days := limits.RetentionDays
	// Extra retention: each retention_pack unit adds retentionPackDays days.
	if sub.AddOns.RetentionPack > 0 {
		days += sub.AddOns.RetentionPack * retentionPackDays
	}
	return days, nil
}
