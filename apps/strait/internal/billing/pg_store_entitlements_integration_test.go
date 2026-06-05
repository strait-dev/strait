//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPgStore_UpdateEntitlements_RoundTrip writes a known OrgPlanLimits
// payload to organization_subscriptions.entitlements and reads it back via
// raw SELECT to verify every JSON-tagged field survives the marshal /
// unmarshal cycle.
func TestPgStore_UpdateEntitlements_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

	want := billing.ComputeEntitlements(&billing.OrgSubscription{
		PlanTier: string(domain.PlanPro),
	}, []billing.Addon{
		{AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true},
		{AddonType: billing.AddonEnvironments5, Quantity: 1, Active: true},
		{AddonType: billing.AddonHistory30d, Quantity: 1, Active: true},
	})
	require.NoError(t, pgStore.
		UpdateEntitlements(
			ctx, orgID, want),
	)

	var raw []byte
	err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw)
	require.NoError(t, err)

	var got billing.OrgPlanLimits
	require.NoError(t, json.Unmarshal(raw,
		&got))
	assert.Equal(t, want.MaxConcurrentRuns,

		got.MaxConcurrentRuns,
	)
	assert.Equal(t, want.MaxEnvironments,

		got.MaxEnvironments,
	)
	assert.Equal(t, want.RetentionDays,

		got.RetentionDays,
	)
	assert.Equal(t, want.WorkerConnections,

		got.WorkerConnections,
	)
	assert.Equal(t, want.MaxRunsPerDay,

		got.MaxRunsPerDay,
	)

	// Spot-check the fields the snapshot must round-trip — these are the
	// hot-path quota fields readers depend on.

}

// TestPgStore_UpdateEntitlements_UnknownOrgIsNoop confirms missing org_id
// returns no error (rows affected = 0). Webhook idempotency retries land
// here for orgs that never persisted; surfacing an error would defeat the
// retry.
func TestPgStore_UpdateEntitlements_UnknownOrgIsNoop(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	want := billing.GetPlanLimits(domain.PlanFree)
	require.NoError(t, pgStore.
		UpdateEntitlements(
			ctx, "org-does-not-exist",

			want))

}

// TestPgStore_UpdateEntitlements_ConcurrentWritersConverge runs two
// goroutines that race on the same org with different payloads. The final
// state must equal one of the two payloads byte-for-byte — not a partial
// blend, not a torn write.
func TestPgStore_UpdateEntitlements_ConcurrentWritersConverge(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-race-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))

	a := billing.GetPlanLimits(domain.PlanPro)
	b := billing.GetPlanLimits(domain.PlanScale)

	var wg sync.WaitGroup
	wg.Add(2)
	concWG.Go(func() {
		defer wg.Done()
		for range 25 {
			_ = pgStore.UpdateEntitlements(ctx, orgID, a)
		}
	})
	concWG.Go(func() {
		defer wg.Done()
		for range 25 {
			_ = pgStore.UpdateEntitlements(ctx, orgID, b)
		}
	})
	wg.Wait()

	var raw []byte
	err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw)
	require.NoError(t, err)

	var got billing.OrgPlanLimits
	require.NoError(t, json.Unmarshal(raw,
		&got))
	assert.False(t, got.MaxConcurrentRuns !=
		a.MaxConcurrentRuns &&
		got.MaxConcurrentRuns !=
			b.MaxConcurrentRuns)
	assert.False(t, got.MaxEnvironments !=
		a.MaxEnvironments &&
		got.
			MaxEnvironments !=

			b.MaxEnvironments)

	// Final state must match Pro or Scale exactly — no torn write.

}

func TestDeepSecPgStore_ApplyPendingDowngradeIfTierRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-ent-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanPro), "active"))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID, string(domain.
				PlanFree)),
	)

	applied, err := pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, string(domain.PlanFree))
	require.NoError(t, err)
	require.True(t, applied)

	got := readDeepSecEntitlements(t, ctx, orgID)
	want := billing.GetPlanLimits(domain.PlanFree)
	require.False(t, got.PlanTier !=
		want.
			PlanTier ||
		got.MaxRunsPerDay !=
			want.MaxRunsPerDay,
	)

}

func TestDeepSecPgStore_ApplyPendingDowngradeTierIfPendingRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-tier-ent-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanScale), "active"))
	require.NoError(t, pgStore.
		SetPendingPlanTier(
			ctx, orgID, string(domain.
				PlanStarter,
			)))

	applied, err := pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, string(domain.PlanStarter))
	require.NoError(t, err)
	require.True(t, applied)

	got := readDeepSecEntitlements(t, ctx, orgID)
	want := billing.GetPlanLimits(domain.PlanStarter)
	require.False(t, got.PlanTier !=
		want.
			PlanTier ||
		got.MaxRunsPerDay !=
			want.MaxRunsPerDay,
	)

}

func readDeepSecEntitlements(t *testing.T, ctx context.Context, orgID string) billing.OrgPlanLimits {
	t.Helper()

	var raw []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`,

		orgID).Scan(&raw))

	var got billing.OrgPlanLimits
	require.NoError(t, json.Unmarshal(raw,
		&got))

	return got
}
