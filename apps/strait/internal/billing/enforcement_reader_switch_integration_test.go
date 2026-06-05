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

// writeRawEntitlements sets the entitlements column directly, bypassing
// PgStore. Used to seed stale or tampered snapshots for the reader switch
// behavior under test.
func writeRawEntitlements(t *testing.T, ctx context.Context, orgID string, raw []byte) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE organization_subscriptions SET entitlements = $2::jsonb WHERE org_id = $1`,
		orgID, raw); err != nil {
		require.Failf(t, "test failure",

			"write raw entitlements for %s: %v", orgID, err)
	}
}

func TestReaderSwitch_IntegrationStaleSnapshotIsAuthoritative(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-stale-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanFree), "active"))

	// Hand-write Enterprise-tier limits to a Free org's row. This is the
	// documented trust boundary: the DB row IS authoritative once the
	// reader switch is on. Anyone with direct DB access can override
	// what the catalog says.
	stale := billing.GetPlanLimits(domain.PlanEnterprise)
	raw, err := json.Marshal(stale)
	require.NoError(t, err)

	writeRawEntitlements(t, ctx, orgID, raw)

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	require.NoError(t, err)

	want := billing.GetPlanLimits(domain.PlanEnterprise)
	assert.Equal(t, want.MaxConcurrentRuns,

		got.MaxConcurrentRuns,
	)

}

func TestReaderSwitch_IntegrationEmptySnapshotPopulatesOpportunistically(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-empty-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanScale), "active"))

	// Nuke the snapshot so the reader has to recompute and write back.
	writeRawEntitlements(t, ctx, orgID, []byte("{}"))

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	if _, err := enforcer.GetOrgPlanLimits(ctx, orgID); err != nil {
		require.Failf(t, "test failure",

			"first GetOrgPlanLimits: %v", err)
	}

	// Direct read: the column is now populated.
	var raw []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, `SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,

		orgID).Scan(&raw))
	assert.False(t, string(raw) == "{}" ||
		len(raw) <= 2)

	var snap billing.OrgPlanLimits
	require.NoError(t, json.Unmarshal(raw,
		&snap))

	want := billing.GetPlanLimits(domain.PlanScale)
	assert.Equal(t, want.MaxConcurrentRuns,

		snap.MaxConcurrentRuns,
	)

}

// TestAdversarialReaderSwitch_MalformedJSONFallsBackAndOverwrites exercises
// the malformed-column path. The reader must NOT panic; it must log, fall
// back to recompute, and overwrite the column with a valid snapshot so
// subsequent reads hit the fast path.
func TestAdversarialReaderSwitch_MalformedJSONFallsBackAndOverwrites(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-bad-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanPro), "active"))

	// Postgres jsonb won't store invalid JSON, so simulate a malformed
	// snapshot by writing a JSON object with values of the wrong type.
	// json.Unmarshal into OrgPlanLimits will fail because PlanTier is
	// expected to be a string but we send a number.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE organization_subscriptions SET entitlements = '{"PlanTier": 12345}'::jsonb WHERE org_id = $1`,
		orgID); err != nil {
		require.Failf(t, "test failure",

			"write malformed: %v", err)
	}

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
	require.NoError(t, err)

	want := billing.GetPlanLimits(domain.PlanPro)
	assert.Equal(t, want.MaxConcurrentRuns,

		got.MaxConcurrentRuns,
	)

	// Column should now be a valid snapshot (overwrite happened).
	var raw []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx, `SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`,

		orgID).Scan(&raw))

	var fixed billing.OrgPlanLimits
	require.NoError(t, json.Unmarshal(raw,
		&fixed),
	)
	assert.Equal(t, want.MaxConcurrentRuns,

		fixed.
			MaxConcurrentRuns,
	)

}

// TestAdversarialReaderSwitch_ConcurrentReadersAndWriters fires N goroutines
// reading limits while M goroutines flip plan tiers. With -race this
// catches any data corruption in the read path; the assertion is just
// that every read returns a coherent snapshot for some valid tier.
func TestAdversarialReaderSwitch_ConcurrentReadersAndWriters(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-rs-race-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx, orgID,
			string(domain.PlanPro), "active"))

	enforcer := billing.NewEnforcer(pgStore, nil, nil,
		billing.WithEntitlementsAuthoritative(true))

	tiers := []domain.PlanTier{
		domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanBusiness,
	}
	validTier := func(p domain.PlanTier) bool {
		for _, t := range tiers {
			if t == p {
				return true
			}
		}
		return p == domain.PlanFree // Free is also valid as catalog default.
	}

	const writers = 4
	const readers = 16
	const reads = 25

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				tier := tiers[i%len(tiers)]
				_ = pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(tier), "active")
			})
		}
	}
	for range readers {
		wg.Add(1)
		concWG.Go(func() {
			defer wg.Done()
			for range reads {
				got, err := enforcer.GetOrgPlanLimits(ctx, orgID)
				assert.NoError(t, err)
				assert.True(t, validTier(
					got.
						PlanTier,
				))

			}
		})
	}
	wg.Wait()
}
