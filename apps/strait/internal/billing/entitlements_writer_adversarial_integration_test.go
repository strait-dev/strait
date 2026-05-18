//go:build integration

package billing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestAdversarialWriter_DuplicateAddonReplay simulates Stripe redelivery of
// the same addon-create event. Idempotency is upstream (processed_webhook_messages)
// so at the store layer a duplicate ID surfaces as an INSERT conflict — but the
// already-persisted snapshot must remain valid (matching the active addon set)
// regardless of the replay outcome.
func TestAdversarialWriter_DuplicateAddonReplay(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-replay-" + newID()
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
		Quantity:  1,
		Active:    true,
	}
	if err := pgStore.CreateAddon(ctx, addon); err != nil {
		t.Fatalf("CreateAddon (first): %v", err)
	}
	first := readEntitlements(t, ctx, orgID)

	// Replay: same ID again. Expect a primary-key violation; the snapshot
	// must NOT be torn — it should still equal the post-first-create state.
	if err := pgStore.CreateAddon(ctx, addon); err == nil {
		t.Fatalf("CreateAddon (replay) expected error, got nil")
	}
	second := readEntitlements(t, ctx, orgID)

	if first.MaxConcurrentRuns != second.MaxConcurrentRuns {
		t.Errorf("snapshot drifted across failed replay: first=%d second=%d",
			first.MaxConcurrentRuns, second.MaxConcurrentRuns)
	}

	// And the resolved snapshot must equal what ComputeEntitlements
	// computes from the actual addon set (one active row, not two).
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	if err != nil {
		t.Fatalf("ListActiveAddons: %v", err)
	}
	want := billing.ComputeEntitlements(sub, addons)
	mustEqualLimits(t, second, want, "after replay")
}

// TestAdversarialWriter_ConcurrentFullUpdates fires N concurrent
// UpdateOrgSubscriptionFull calls for the same org with different plan tiers.
// The final persisted entitlements must equal a valid serialization of one
// of the inputs — never a torn write blending tiers.
func TestAdversarialWriter_ConcurrentFullUpdates(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-race-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}

	tiers := []domain.PlanTier{
		domain.PlanStarter,
		domain.PlanPro,
		domain.PlanScale,
		domain.PlanBusiness,
	}
	now := time.Now().UTC()
	pe := now.Add(30 * 24 * time.Hour)

	var wg sync.WaitGroup
	for _, tier := range tiers {
		wg.Add(1)
		go func(tier domain.PlanTier) {
			defer wg.Done()
			_ = pgStore.UpdateOrgSubscriptionFull(ctx, orgID, string(tier), "active", &now, &pe)
		}(tier)
	}
	wg.Wait()

	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSubscription: %v", err)
	}
	addons, err := pgStore.ListActiveAddons(ctx, orgID)
	if err != nil {
		t.Fatalf("ListActiveAddons: %v", err)
	}
	want := billing.ComputeEntitlements(sub, addons)
	got := readEntitlements(t, ctx, orgID)

	// Final snapshot must equal ComputeEntitlements over the final row.
	// (Refresh-after-write means whichever UPDATE landed last sets both
	// the row and the snapshot; even if a slow refresh fires after a
	// later mutator's refresh, both reads come from a coherent state.)
	mustEqualLimits(t, got, want, "after concurrent full updates")

	// And the row-resolved tier must be one of the candidates, not zero.
	matched := false
	for _, tier := range tiers {
		if sub.PlanTier == string(tier) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("final plan_tier %q not one of candidates %v", sub.PlanTier, tiers)
	}
}

// TestAdversarialWriter_DeactivateThenCreateRefreshes covers the addon
// deactivate -> create cycle. The snapshot must reflect each transition;
// a stale snapshot from before the deactivate must not survive.
func TestAdversarialWriter_DeactivateThenCreateRefreshes(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-cycle-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}

	a := &billing.Addon{ID: newID(), OrgID: orgID, AddonType: billing.AddonConcurrency100, Quantity: 3, Active: true}
	if err := pgStore.CreateAddon(ctx, a); err != nil {
		t.Fatalf("CreateAddon a: %v", err)
	}
	withA := readEntitlements(t, ctx, orgID)

	if err := pgStore.DeactivateAddon(ctx, a.ID); err != nil {
		t.Fatalf("DeactivateAddon a: %v", err)
	}
	postDeactivate := readEntitlements(t, ctx, orgID)
	if postDeactivate.MaxConcurrentRuns >= withA.MaxConcurrentRuns {
		t.Errorf("snapshot did not collapse after deactivate: with=%d after=%d",
			withA.MaxConcurrentRuns, postDeactivate.MaxConcurrentRuns)
	}

	b := &billing.Addon{ID: newID(), OrgID: orgID, AddonType: billing.AddonConcurrency100, Quantity: 1, Active: true}
	if err := pgStore.CreateAddon(ctx, b); err != nil {
		t.Fatalf("CreateAddon b: %v", err)
	}
	withB := readEntitlements(t, ctx, orgID)
	if withB.MaxConcurrentRuns != postDeactivate.MaxConcurrentRuns+100 {
		t.Errorf("after second create: got %d, want %d",
			withB.MaxConcurrentRuns, postDeactivate.MaxConcurrentRuns+100)
	}
}

// TestAdversarialWriter_RestrictThenResume covers the trust-boundary
// case where an org gets restricted (snapshot collapses to Free) and is
// later resumed via UpdateOrgSubscriptionFull. The post-resume snapshot
// must reflect the new tier, not the stale Free snapshot.
func TestAdversarialWriter_RestrictThenResume(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-adv-resume-" + newID()
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
	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanFree), "after restrict")

	// Resume: customer pays, plan goes back to Pro.
	now := time.Now().UTC()
	pe := now.Add(30 * 24 * time.Hour)
	if err := pgStore.UpdateOrgSubscriptionFull(ctx, orgID, string(domain.PlanPro), "active", &now, &pe); err != nil {
		t.Fatalf("UpdateOrgSubscriptionFull (resume): %v", err)
	}
	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "after resume")
}

func TestAdversarialWriter_PaymentRestrictionRefreshesEntitlements(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-payment-restrict-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("EnsureOrgSubscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("UpdateOrgSubscriptionPlan: %v", err)
	}
	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "before payment restriction")

	if err := pgStore.UpdatePaymentStatus(ctx, orgID, "restricted", nil); err != nil {
		t.Fatalf("UpdatePaymentStatus(restricted): %v", err)
	}
	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanFree), "restricted payment status")

	if err := pgStore.UpdatePaymentStatus(ctx, orgID, "ok", nil); err != nil {
		t.Fatalf("UpdatePaymentStatus(ok): %v", err)
	}
	mustEqualLimits(t, readEntitlements(t, ctx, orgID),
		billing.GetPlanLimits(domain.PlanPro), "payment status restored")
}
