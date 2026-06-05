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
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}

	want := billing.ComputeEntitlements(&billing.OrgSubscription{
		PlanTier: string(domain.PlanPro),
	}, []billing.Addon{
		{AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true},
		{AddonType: billing.AddonEnvironments5, Quantity: 1, Active: true},
		{AddonType: billing.AddonHistory30d, Quantity: 1, Active: true},
	})

	if err := pgStore.UpdateEntitlements(ctx, orgID, want); err != nil {
		t.Fatalf("UpdateEntitlements: %v", err)
	}

	var raw []byte
	err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw)
	if err != nil {
		t.Fatalf("read raw entitlements: %v", err)
	}

	var got billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entitlements: %v", err)
	}

	// Spot-check the fields the snapshot must round-trip — these are the
	// hot-path quota fields readers depend on.
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns {
		t.Errorf("MaxConcurrentRuns: got %d, want %d", got.MaxConcurrentRuns, want.MaxConcurrentRuns)
	}
	if got.MaxEnvironments != want.MaxEnvironments {
		t.Errorf("MaxEnvironments: got %d, want %d", got.MaxEnvironments, want.MaxEnvironments)
	}
	if got.RetentionDays != want.RetentionDays {
		t.Errorf("RetentionDays: got %d, want %d", got.RetentionDays, want.RetentionDays)
	}
	if got.WorkerConnections != want.WorkerConnections {
		t.Errorf("WorkerConnections: got %d, want %d", got.WorkerConnections, want.WorkerConnections)
	}
	if got.MaxRunsPerDay != want.MaxRunsPerDay {
		t.Errorf("MaxRunsPerDay: got %d, want %d", got.MaxRunsPerDay, want.MaxRunsPerDay)
	}
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
	if err := pgStore.UpdateEntitlements(ctx, "org-does-not-exist", want); err != nil {
		t.Fatalf("UpdateEntitlements on unknown org: got error %v, want nil", err)
	}
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
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}

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
	if err != nil {
		t.Fatalf("read entitlements: %v", err)
	}

	var got billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entitlements: %v", err)
	}

	// Final state must match Pro or Scale exactly — no torn write.
	if got.MaxConcurrentRuns != a.MaxConcurrentRuns && got.MaxConcurrentRuns != b.MaxConcurrentRuns {
		t.Errorf("torn write: MaxConcurrentRuns=%d matches neither payload (a=%d, b=%d)",
			got.MaxConcurrentRuns, a.MaxConcurrentRuns, b.MaxConcurrentRuns)
	}
	if got.MaxEnvironments != a.MaxEnvironments && got.MaxEnvironments != b.MaxEnvironments {
		t.Errorf("torn write: MaxEnvironments=%d matches neither payload (a=%d, b=%d)",
			got.MaxEnvironments, a.MaxEnvironments, b.MaxEnvironments)
	}
}

func TestDeepSecPgStore_ApplyPendingDowngradeIfTierRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-ent-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, string(domain.PlanFree)); err != nil {
		t.Fatalf("SetPendingPlanTier: %v", err)
	}

	applied, err := pgStore.ApplyPendingDowngradeIfTier(ctx, orgID, string(domain.PlanFree))
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeIfTier: %v", err)
	}
	if !applied {
		t.Fatal("expected pending downgrade to apply")
	}

	got := readDeepSecEntitlements(t, ctx, orgID)
	want := billing.GetPlanLimits(domain.PlanFree)
	if got.PlanTier != want.PlanTier || got.MaxRunsPerDay != want.MaxRunsPerDay {
		t.Fatalf("entitlements not refreshed to free tier: got tier=%q max_runs=%d want tier=%q max_runs=%d",
			got.PlanTier, got.MaxRunsPerDay, want.PlanTier, want.MaxRunsPerDay)
	}
}

func TestDeepSecPgStore_ApplyPendingDowngradeTierIfPendingRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-pending-tier-ent-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanScale), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, string(domain.PlanStarter)); err != nil {
		t.Fatalf("SetPendingPlanTier: %v", err)
	}

	applied, err := pgStore.ApplyPendingDowngradeTierIfPending(ctx, orgID, string(domain.PlanStarter))
	if err != nil {
		t.Fatalf("ApplyPendingDowngradeTierIfPending: %v", err)
	}
	if !applied {
		t.Fatal("expected pending tier to apply")
	}

	got := readDeepSecEntitlements(t, ctx, orgID)
	want := billing.GetPlanLimits(domain.PlanStarter)
	if got.PlanTier != want.PlanTier || got.MaxRunsPerDay != want.MaxRunsPerDay {
		t.Fatalf("entitlements not refreshed to starter tier: got tier=%q max_runs=%d want tier=%q max_runs=%d",
			got.PlanTier, got.MaxRunsPerDay, want.PlanTier, want.MaxRunsPerDay)
	}
}

func readDeepSecEntitlements(t *testing.T, ctx context.Context, orgID string) billing.OrgPlanLimits {
	t.Helper()

	var raw []byte
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw); err != nil {
		t.Fatalf("read entitlements: %v", err)
	}
	var got billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entitlements: %v", err)
	}
	return got
}
