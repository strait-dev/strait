//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// readEntitlements pulls the persisted snapshot and unmarshals it. Used by
// every test below to assert the writer-side wiring populated the column.
func readEntitlements(t *testing.T, ctx context.Context, orgID string) billing.OrgPlanLimits {
	t.Helper()
	var raw []byte
	err := testDB.Pool.QueryRow(ctx, `
		SELECT entitlements FROM organization_subscriptions WHERE org_id = $1
	`, orgID).Scan(&raw)
	if err != nil {
		t.Fatalf("read entitlements for %s: %v", orgID, err)
	}
	var got billing.OrgPlanLimits
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entitlements for %s: %v", orgID, err)
	}
	return got
}

func mustEqualLimits(t *testing.T, got, want billing.OrgPlanLimits, label string) {
	t.Helper()
	if got.MaxConcurrentRuns != want.MaxConcurrentRuns ||
		got.MaxEnvironments != want.MaxEnvironments ||
		got.RetentionDays != want.RetentionDays ||
		got.MaxRunsPerDay != want.MaxRunsPerDay ||
		got.WorkerConnections != want.WorkerConnections {
		t.Errorf("%s: snapshot drift\n got:  %+v\n want: %+v", label, got, want)
	}
}

// TestEntitlementsWriter_UpsertPopulatesSnapshot exercises the
// UpsertOrgSubscription path. The snapshot must be populated immediately
// after the mutator returns.
func TestEntitlementsWriter_UpsertPopulatesSnapshot(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-upsert-" + newID()
	now := time.Now().UTC().Truncate(time.Microsecond)
	sub := &billing.OrgSubscription{
		ID:        newID(),
		OrgID:     orgID,
		PlanTier:  string(domain.PlanPro),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := pgStore.UpsertOrgSubscription(ctx, sub); err != nil {
		t.Fatalf("UpsertOrgSubscription: %v", err)
	}

	got := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, got, billing.GetPlanLimits(domain.PlanPro), "after upsert")
}

// TestEntitlementsWriter_PlanUpdateRefreshesSnapshot exercises
// UpdateOrgSubscriptionPlan. Going Pro -> Scale must rewrite the snapshot
// to Scale-tier limits.
func TestEntitlementsWriter_PlanUpdateRefreshesSnapshot(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-planupd-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanScale), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}

	got := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, got, billing.GetPlanLimits(domain.PlanScale), "after plan update")
}

// TestEntitlementsWriter_FullUpdateRefreshesSnapshot exercises
// UpdateOrgSubscriptionFull (plan + period dates).
func TestEntitlementsWriter_FullUpdateRefreshesSnapshot(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-full-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	now := time.Now().UTC()
	pe := now.Add(30 * 24 * time.Hour)
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, string(domain.PlanBusiness), "active", &now, &pe); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull: %v", err)
	}

	got := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, got, billing.GetPlanLimits(domain.PlanBusiness), "after full update")
}

// TestEntitlementsWriter_AddonCreateThenDeactivate exercises the addon
// CRUD path. After CreateAddon the snapshot must reflect the addon; after
// DeactivateAddon it must collapse back to base limits.
func TestEntitlementsWriter_AddonCreateThenDeactivate(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-addon-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}

	addon := &billing.Addon{
		ID:        newID(),
		OrgID:     orgID,
		AddonType: billing.AddonConcurrency100,
		Quantity:  2,
		Active:    true,
	}
	if err := pgStore.CreateAddon(ctx, addon); err != nil {
		t.Fatalf("CreateAddon: %v", err)
	}

	withAddon := readEntitlements(t, ctx, orgID)
	wantWith := billing.GetPlanLimits(domain.PlanPro)
	wantWith.MaxConcurrentRuns += 200 // 2 packs of 100
	if withAddon.MaxConcurrentRuns != wantWith.MaxConcurrentRuns {
		t.Errorf("after CreateAddon: MaxConcurrentRuns got %d, want %d",
			withAddon.MaxConcurrentRuns, wantWith.MaxConcurrentRuns)
	}

	if err := pgStore.DeactivateAddon(ctx, addon.ID); err != nil {
		t.Fatalf("DeactivateAddon: %v", err)
	}
	withoutAddon := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, withoutAddon, billing.GetPlanLimits(domain.PlanPro), "after addon deactivate")
}

// TestEntitlementsWriter_RestrictOrgCollapsesToFree exercises RestrictOrgTx.
// The snapshot must reflect Free-tier limits the moment the transaction
// commits, not on the next mutator.
func TestEntitlementsWriter_RestrictOrgCollapsesToFree(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-restrict-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}

	graceEnd := time.Now().Add(7 * 24 * time.Hour)
	if err := billing.RestrictOrgTx(ctx, testDB.Pool, orgID, &graceEnd); err != nil {
		t.Fatalf("RestrictOrgTx: %v", err)
	}

	got := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, got, billing.GetPlanLimits(domain.PlanFree), "after restrict")
}

// TestEntitlementsWriter_ApplyPendingDowngradeRefreshesSnapshot exercises
// the scheduled downgrade pathway. After ApplyPendingDowngrade the
// snapshot must reflect the new (downgraded) tier.
func TestEntitlementsWriter_ApplyPendingDowngradeRefreshesSnapshot(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-downgrade-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}
	if err := pgStore.SetPendingPlanTier(ctx, orgID, string(domain.PlanStarter)); err != nil {
		t.Fatalf("SetPendingPlanTier: %v", err)
	}
	if err := pgStore.ApplyPendingDowngrade(ctx, orgID); err != nil {
		t.Fatalf("ApplyPendingDowngrade: %v", err)
	}

	got := readEntitlements(t, ctx, orgID)
	mustEqualLimits(t, got, billing.GetPlanLimits(domain.PlanStarter), "after apply pending downgrade")
}

// TestEntitlementsWriter_SnapshotEqualsComputeEntitlements is the bedrock
// invariant: at every step in a longer sequence, the persisted snapshot
// must equal what ComputeEntitlements would return for the same state.
// Drift here means a writer forgot to refresh entitlements; this test catches
// it for the writer side specifically.
func TestEntitlementsWriter_SnapshotEqualsComputeEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-ent-invariant-" + newID()
	steps := []func() error{
		func() error { return pgStore.EnsureOrgSubscription(ctx, orgID) },
		func() error { return pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active") },
		func() error {
			return pgStore.CreateAddon(ctx, &billing.Addon{
				ID: newID(), OrgID: orgID, AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true,
			})
		},
		func() error { return pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanScale), "active") },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		sub, err := pgStore.GetOrgSubscription(ctx, orgID)
		if err != nil {
			t.Fatalf("step %d GetOrgSubscription: %v", i, err)
		}
		addons, err := pgStore.ListActiveAddons(ctx, orgID)
		if err != nil {
			t.Fatalf("step %d ListActiveAddons: %v", i, err)
		}
		want := billing.ComputeEntitlements(sub, addons)
		got := readEntitlements(t, ctx, orgID)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("step %d: snapshot != ComputeEntitlements\n got:  %+v\n want: %+v", i, got, want)
		}
	}
}
