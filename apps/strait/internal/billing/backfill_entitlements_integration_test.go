//go:build integration

package billing_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// seedSubsForBackfill creates n org_subscriptions rows with mixed tiers and
// blanks the entitlements column on each so the backfill has work to do.
// Returns the slice of orgIDs in seeded order.
func seedSubsForBackfill(t *testing.T, ctx context.Context, pgStore *billing.PgStore, n int) []string {
	t.Helper()
	tiers := []domain.PlanTier{
		domain.PlanFree, domain.PlanStarter, domain.PlanPro,
		domain.PlanScale, domain.PlanBusiness,
	}
	ids := make([]string, n)
	for i := range n {
		id := "org-bf-" + newID()
		ids[i] = id
		if err := pgStore.EnsureOrgSubscription(ctx, id); err != nil {
			t.Fatalf("ensure %s: %v", id, err)
		}
		tier := tiers[i%len(tiers)]
		if tier != domain.PlanFree {
			if err := pgStore.UpdateOrgSubscriptionPlan(ctx, id, string(tier), "active"); err != nil {
				t.Fatalf("update plan %s: %v", id, err)
			}
		}
		// Seed an addon on every third row to exercise the addon path.
		if i%3 == 0 && tier != domain.PlanFree {
			if err := pgStore.CreateAddon(ctx, &billing.Addon{
				ID: newID(), OrgID: id, AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true,
			}); err != nil {
				t.Fatalf("create addon %s: %v", id, err)
			}
		}
	}
	// Blank entitlements on all rows so the backfill has work to do.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE organization_subscriptions SET entitlements = '{}'::jsonb`); err != nil {
		t.Fatalf("blank entitlements: %v", err)
	}
	return ids
}

func TestBackfillEntitlements_PopulatesAllRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	n := 50
	ids := seedSubsForBackfill(t, ctx, pgStore, n)

	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 7, false, "", nil)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if stats.Scanned != int64(n) {
		t.Errorf("scanned = %d, want %d", stats.Scanned, n)
	}
	if stats.Updated != int64(n) {
		t.Errorf("updated = %d, want %d", stats.Updated, n)
	}

	// Every row's snapshot must equal ComputeEntitlements over its current state.
	for _, id := range ids {
		sub, err := pgStore.GetOrgSubscription(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		addons, err := pgStore.ListActiveAddons(ctx, id)
		if err != nil {
			t.Fatalf("list addons %s: %v", id, err)
		}
		want := billing.ComputeEntitlements(sub, addons)
		got := readEntitlements(t, ctx, id)
		mustEqualLimits(t, got, want, "after backfill: "+id)
	}
}

func TestBackfillEntitlements_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	seedSubsForBackfill(t, ctx, pgStore, 20)

	// First run writes everything.
	first, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil)
	if err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if first.Updated != 20 {
		t.Errorf("first run updated = %d, want 20", first.Updated)
	}
	// Second run is a no-op because the snapshots already match.
	second, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if second.Updated != 0 {
		t.Errorf("second run updated = %d, want 0 (idempotent)", second.Updated)
	}
	if second.Scanned != 20 {
		t.Errorf("second run scanned = %d, want 20", second.Scanned)
	}
}

func TestBackfillEntitlements_DryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ids := seedSubsForBackfill(t, ctx, pgStore, 10)

	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, true, "", nil)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if stats.Updated != 10 {
		t.Errorf("dry-run reported %d would-update, want 10", stats.Updated)
	}
	// Column must still be blank.
	for _, id := range ids {
		var raw []byte
		if err := testDB.Pool.QueryRow(ctx,
			`SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`, id).Scan(&raw); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if string(raw) != "{}" {
			t.Errorf("dry-run wrote to %s: got %q, want {}", id, string(raw))
		}
	}
}

func TestBackfillEntitlements_SingleOrgScope(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	ids := seedSubsForBackfill(t, ctx, pgStore, 5)

	target := ids[2]
	stats, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 100, false, target, nil)
	if err != nil {
		t.Fatalf("single-org backfill: %v", err)
	}
	if stats.Scanned != 1 || stats.Updated != 1 {
		t.Errorf("single-org stats = %+v, want {1, 1}", stats)
	}

	// Target row populated; others still blank.
	for i, id := range ids {
		var raw []byte
		if err := testDB.Pool.QueryRow(ctx,
			`SELECT entitlements FROM organization_subscriptions WHERE org_id = $1`, id).Scan(&raw); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if i == 2 {
			if string(raw) == "{}" {
				t.Errorf("target row not populated: %s", id)
			}
		} else if string(raw) != "{}" {
			t.Errorf("non-target row %s was touched: %q", id, string(raw))
		}
	}
}

func TestUpdateEntitlementsIfUnchanged_SkipsStaleBackfillWrite(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-backfill-freshness-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan(pro): %v", err)
	}
	observed, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	staleProSnapshot := billing.GetPlanLimits(domain.PlanPro)

	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanFree), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan(free): %v", err)
	}
	updated, err := billing.UpdateEntitlementsIfUnchanged(ctx, testDB.Pool, orgID, staleProSnapshot, observed.UpdatedAt)
	if err != nil {
		t.Fatalf("UpdateEntitlementsIfUnchanged: %v", err)
	}
	if updated {
		t.Fatal("stale backfill write updated row after live plan change")
	}

	got := readEntitlements(t, ctx, orgID)
	if got.PlanTier != domain.PlanFree {
		t.Fatalf("persisted plan tier = %s, want %s", got.PlanTier, domain.PlanFree)
	}
}

// TestBackfillEntitlements_AdversarialConcurrentWebhookWriter races the
// backfill against a concurrent UpdateOrgSubscriptionPlan loop that keeps
// flipping plan tiers on the same orgs. Final state must equal
// ComputeEntitlements over whatever state landed last — no torn writes.
func TestBackfillEntitlements_AdversarialConcurrentWebhookWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)
	ids := seedSubsForBackfill(t, ctx, pgStore, 30)

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tiers := []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale}
		i := 0
		for !stop.Load() {
			id := ids[i%len(ids)]
			tier := tiers[i%len(tiers)]
			_ = pgStore.UpdateOrgSubscriptionPlan(ctx, id, string(tier), "active")
			i++
		}
	}()

	// Run the backfill while writers churn.
	if _, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil); err != nil {
		stop.Store(true)
		wg.Wait()
		t.Fatalf("backfill: %v", err)
	}
	stop.Store(true)
	wg.Wait()

	// One final consistent backfill pass to settle everything.
	if _, err := billing.BackfillEntitlements(ctx, testDB.Pool, pgStore, 5, false, "", nil); err != nil {
		t.Fatalf("final backfill: %v", err)
	}

	// Now every row's snapshot must equal what ComputeEntitlements returns
	// for its current state.
	for _, id := range ids {
		sub, err := pgStore.GetOrgSubscription(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		addons, err := pgStore.ListActiveAddons(ctx, id)
		if err != nil {
			t.Fatalf("list addons %s: %v", id, err)
		}
		want := billing.ComputeEntitlements(sub, addons)
		got := readEntitlements(t, ctx, id)
		mustEqualLimits(t, got, want, "post-race: "+id)
	}
}
