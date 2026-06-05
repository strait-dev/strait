package billing

import "strait/internal/domain"

// ComputeEntitlements resolves the authoritative plan limits for a subscription
// by composing the static plan catalog and table-backed launch add-ons. The
// legacy subscription-row JSONB add_ons column is retained as an inert
// compatibility step. The output is the value to persist to
// organization_subscriptions.entitlements so subsequent reads can skip the
// 3-step recompute pipeline.
//
// The composition order matches Enforcer.GetOrgPlanLimits:
//
//  1. GetPlanLimits(sub.PlanTier)           — static catalog baseline.
//  2. EffectiveLimits(base, addons)         — table-driven addons.
//  3. ApplySubscriptionAddOns(base, addOns) — inert legacy JSONB compatibility.
//
// A nil sub falls back to the Free-tier baseline. Per-org support overrides
// (override_daily_run_limit, override_concurrent_run_limit) live on the same
// row and are intentionally NOT folded into the snapshot — those are runtime
// knobs operators flip without rerunning the entitlements writer.
func ComputeEntitlements(sub *OrgSubscription, addons []Addon) OrgPlanLimits {
	if sub == nil {
		return GetPlanLimits(domain.PlanFree)
	}

	// Restricted orgs collapse to base Free regardless of plan tier or
	// active addons — restriction is a hard kill switch, not a soft tier
	// change. RestrictOrgTx writes the same shape directly; this branch
	// keeps ComputeEntitlements in sync so the consistency invariant
	// (snapshot == ComputeEntitlements(sub, addons)) holds globally.
	// Only the subscription Status is checked: PaymentStatus="restricted"
	// can persist after a successful resume webhook clears Status back to
	// "active", and we don't want that lingering flag to suppress the new
	// tier's entitlements.
	if sub.Status == "restricted" || sub.Status == "paused" || sub.PaymentStatus == "restricted" {
		return GetPlanLimits(domain.PlanFree)
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	if len(addons) > 0 {
		limits = EffectiveLimits(limits, addons)
	}
	limits = ApplySubscriptionAddOns(limits, sub.AddOns)
	return limits
}
