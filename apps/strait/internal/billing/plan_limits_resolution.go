package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"
)

type orgPlanLimitsResolution struct {
	tier            domain.PlanTier
	limits          OrgPlanLimits
	enforcementMode string
	cacheVersion    int64
}

func (e *Enforcer) resolveOrgPlanLimits(
	ctx context.Context,
	orgID string,
	sub *OrgSubscription,
) (orgPlanLimitsResolution, error) {
	tier := domain.PlanTier(sub.PlanTier)
	cacheVersion := orgSubscriptionCacheVersion(sub)
	limits, usedSnapshot := e.limitsFromEntitlementsSnapshot(orgID, sub)

	if !usedSnapshot {
		var err error
		limits, err = e.computeOrgPlanLimits(ctx, orgID, tier, sub)
		if err != nil {
			return orgPlanLimitsResolution{}, err
		}
		if e.entitlementsAuthoritative {
			if err := e.store.UpdateEntitlements(ctx, orgID, limits); err != nil {
				e.logger.Warn("failed to opportunistically populate entitlements",
					"org_id", orgID, "error", err)
			} else {
				cacheVersion++
			}
		}
	}

	applyOrgLimitOverrides(&limits, sub)
	return orgPlanLimitsResolution{
		tier:            tier,
		limits:          limits,
		enforcementMode: sub.EnforcementMode,
		cacheVersion:    cacheVersion,
	}, nil
}

func (e *Enforcer) limitsFromEntitlementsSnapshot(
	orgID string,
	sub *OrgSubscription,
) (OrgPlanLimits, bool) {
	if !e.entitlementsAuthoritative || !hasPersistedEntitlements(sub.Entitlements) {
		return OrgPlanLimits{}, false
	}
	var snap OrgPlanLimits
	if err := json.Unmarshal(sub.Entitlements, &snap); err != nil {
		e.logger.Warn("entitlements snapshot is malformed, falling back to recompute",
			"org_id", orgID, "error", err)
		return OrgPlanLimits{}, false
	}
	return snap, true
}

func (e *Enforcer) computeOrgPlanLimits(
	ctx context.Context,
	orgID string,
	tier domain.PlanTier,
	sub *OrgSubscription,
) (OrgPlanLimits, error) {
	limits := GetPlanLimits(tier)

	addons, addonErr := e.store.ListActiveAddons(ctx, orgID)
	if addonErr != nil {
		return OrgPlanLimits{}, fmt.Errorf("listing active add-ons: %w", addonErr)
	} else if len(addons) > 0 {
		limits = EffectiveLimits(limits, addons)
	}

	return ApplySubscriptionAddOns(limits, sub.AddOns), nil
}

func applyOrgLimitOverrides(limits *OrgPlanLimits, sub *OrgSubscription) {
	// The legacy daily run override is intentionally ignored for launch:
	// billing is monthly orchestration runs, and all launch plans keep
	// MaxRunsPerDay at -1 so stale support metadata cannot reactivate a
	// public daily quota.
	if sub.OverrideConcurrentRunLimit != nil {
		limits.MaxConcurrentRuns = *sub.OverrideConcurrentRunLimit
	}
}

func orgSubscriptionCacheVersion(sub *OrgSubscription) int64 {
	if sub == nil || sub.CacheVersion <= 0 {
		return 1
	}
	return sub.CacheVersion
}

// hasPersistedEntitlements reports whether the raw JSONB bytes contain
// a non-default snapshot. The migration default is the empty object `{}`
// which is two bytes; anything longer is treated as a populated snapshot.
// nil and zero-length are also considered empty.
func hasPersistedEntitlements(raw []byte) bool {
	if len(raw) <= 2 {
		return false
	}
	// Tolerate whitespace-only payloads like ` {} ` by trimming and
	// comparing to the empty object literal.
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 2 && string(trimmed) != "{}"
}
