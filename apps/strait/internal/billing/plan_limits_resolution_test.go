package billing

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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

	resolution, err := e.resolveOrgPlanLimits(ctx, sub.OrgID, sub)
	require.NoError(t,
		err)
	require.EqualValues(t, 12, sub.CacheVersion)
	require.EqualValues(t, 13, resolution.
		cacheVersion)

	if _, ok := store.lastEntitlementsUpdates[sub.OrgID]; !ok {
		require.Failf(t, "test failure",

			"expected opportunistic entitlements write for %s", sub.OrgID)
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

	resolution, err := e.resolveOrgPlanLimits(ctx, sub.OrgID, sub)
	require.NoError(t,
		err)
	require.EqualValues(t, -1, resolution.
		limits.MaxRunsPerDay)
	require.Equal(t,
		overrideConcurrent,

		resolution.limits.
			MaxConcurrentRuns,
	)

	persisted, ok := store.lastEntitlementsUpdates[sub.OrgID]
	require.True(t, ok)

	base := GetPlanLimits(domain.PlanPro)
	require.Equal(t,
		base.MaxRunsPerDay,

		persisted.MaxRunsPerDay,
	)
	require.Equal(t,
		base.MaxConcurrentRuns,

		persisted.MaxConcurrentRuns,
	)
}
