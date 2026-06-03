package billing

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestResolveOrgPlanLimits_DoesNotMutateSubscriptionCacheVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)
	sub := &OrgSubscription{
		ID:           "sub",
		OrgID:        "org-version-local",
		PlanTier:     string(domain.PlanPro),
		Status:       "active",
		Entitlements: []byte("{}"),
		CacheVersion: 12,
	}

	resolution := e.resolveOrgPlanLimits(ctx, sub.OrgID, sub)

	if sub.CacheVersion != 12 {
		t.Fatalf("subscription CacheVersion mutated to %d, want 12", sub.CacheVersion)
	}
	if resolution.cacheVersion != 13 {
		t.Fatalf("resolution cacheVersion = %d, want 13", resolution.cacheVersion)
	}
	if _, ok := store.lastEntitlementsUpdates[sub.OrgID]; !ok {
		t.Fatalf("expected opportunistic entitlements write for %s", sub.OrgID)
	}
}

func TestResolveOrgPlanLimits_OverridesStayOutOfPersistedSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, store := makeReaderSwitchEnforcer(t, true)
	overrideDaily := 7
	overrideConcurrent := 3
	sub := &OrgSubscription{
		ID:                         "sub",
		OrgID:                      "org-overrides",
		PlanTier:                   string(domain.PlanPro),
		Status:                     "active",
		Entitlements:               []byte("{}"),
		OverrideDailyRunLimit:      &overrideDaily,
		OverrideConcurrentRunLimit: &overrideConcurrent,
	}

	resolution := e.resolveOrgPlanLimits(ctx, sub.OrgID, sub)

	if resolution.limits.MaxRunsPerDay != int64(overrideDaily) {
		t.Fatalf("resolved MaxRunsPerDay = %d, want override %d", resolution.limits.MaxRunsPerDay, overrideDaily)
	}
	if resolution.limits.MaxConcurrentRuns != overrideConcurrent {
		t.Fatalf("resolved MaxConcurrentRuns = %d, want override %d", resolution.limits.MaxConcurrentRuns, overrideConcurrent)
	}
	persisted, ok := store.lastEntitlementsUpdates[sub.OrgID]
	if !ok {
		t.Fatalf("expected opportunistic entitlements write for %s", sub.OrgID)
	}
	base := GetPlanLimits(domain.PlanPro)
	if persisted.MaxRunsPerDay != base.MaxRunsPerDay {
		t.Fatalf("persisted MaxRunsPerDay = %d, want base %d", persisted.MaxRunsPerDay, base.MaxRunsPerDay)
	}
	if persisted.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Fatalf("persisted MaxConcurrentRuns = %d, want base %d", persisted.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
}
