package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Grace period tests for Scale tier.

func TestGracePeriod_ScaleTier_BlockAfterExpiry(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	expired := time.Now().Add(-1 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-scale-expired": {
				OrgID:          "org-scale-expired",
				PlanTier:       "scale",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &expired,
			},
		},
	}

	enforcer := NewEnforcer(store, rdb, slog.Default())
	err := enforcer.CheckDailyRunLimit(context.Background(), "org-scale-expired")
	if err == nil {
		t.Fatal("expected block after grace period expiry for Scale tier")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "grace_period_expired" {
		t.Errorf("Code = %q, want grace_period_expired", le.Code)
	}
}

func TestGracePeriod_ScaleTier_AllowDuringGrace(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	future := time.Now().Add(24 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-scale-grace": {
				OrgID:          "org-scale-grace",
				PlanTier:       "scale",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &future,
			},
		},
	}

	enforcer := NewEnforcer(store, rdb, slog.Default())
	err := enforcer.CheckDailyRunLimit(context.Background(), "org-scale-grace")
	if err != nil {
		t.Fatalf("expected runs allowed during active grace period: %v", err)
	}
}

func TestGracePeriod_ScaleTier_PaymentRestricted(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-scale-restricted": {
				OrgID:         "org-scale-restricted",
				PlanTier:      "scale",
				Status:        "active",
				PaymentStatus: "restricted",
			},
		},
	}

	enforcer := NewEnforcer(store, rdb, slog.Default())
	err := enforcer.CheckDailyRunLimit(context.Background(), "org-scale-restricted")
	if err == nil {
		t.Fatal("expected block when payment is restricted")
	}
	var le *LimitError
	if !isLimitError(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "payment_restricted" {
		t.Errorf("Code = %q, want payment_restricted", le.Code)
	}
}

// Idempotent addon deactivation.

func TestDeactivateAddon_DoubleCall_NoError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}

	// Create an addon.
	addon := &Addon{
		ID:        "addon-1",
		OrgID:     "org-1",
		AddonType: AddonConcurrency100,
		Quantity:  1,
		Active:    true,
	}
	if err := store.CreateAddon(context.Background(), addon); err != nil {
		t.Fatalf("CreateAddon failed: %v", err)
	}

	// Deactivate once.
	if err := store.DeactivateAddon(context.Background(), "addon-1"); err != nil {
		t.Fatalf("first DeactivateAddon failed: %v", err)
	}

	// Deactivate again -- should be idempotent, no error.
	if err := store.DeactivateAddon(context.Background(), "addon-1"); err != nil {
		t.Fatalf("second DeactivateAddon failed: %v", err)
	}
}

// Self-hosted verification: billing gates are skipped, but the launch catalog
// still distinguishes active entitlements from roadmap/contact-sales features.

func TestSelfHosted_LaunchActiveFeaturesAvailable(t *testing.T) {
	t.Parallel()

	// On self-hosted, the enforcer is nil and getOrgPlanLimits short-circuits.
	// The key invariant: every plan gate check returns nil when limits are nil.
	// This test verifies the registry independently allows launch-active
	// features on Enterprise while keeping roadmap features inactive.
	reg := NewStaticRegistry()

	enterpriseFeatures := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSLA,
	}

	for _, f := range enterpriseFeatures {
		if !reg.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should have feature %q", f)
		}
	}

	for _, f := range roadmapEnterpriseFeatures {
		if reg.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should not have launch-roadmap feature %q", f)
		}
	}

	enterpriseLimits := GetPlanLimits(domain.PlanEnterprise)

	// Verify unlimited limits.
	if enterpriseLimits.MaxConcurrentRuns != -1 {
		t.Errorf("Enterprise MaxConcurrentRuns = %d, want -1", enterpriseLimits.MaxConcurrentRuns)
	}
	if enterpriseLimits.MaxWorkflowDAGSteps != -1 {
		t.Errorf("Enterprise MaxWorkflowDAGSteps = %d, want -1", enterpriseLimits.MaxWorkflowDAGSteps)
	}
	if enterpriseLimits.MaxScheduledJobs != -1 {
		t.Errorf("Enterprise MaxScheduledJobs = %d, want -1", enterpriseLimits.MaxScheduledJobs)
	}
}

func TestSelfHosted_EditionCommunity_SkipsGating(t *testing.T) {
	t.Parallel()

	// EditionCommunity should not require HTTP mode gating.
	edition := domain.EditionCommunity
	if edition.RequiresHTTPModeGating() {
		t.Error("EditionCommunity.RequiresHTTPModeGating() = true, want false")
	}

	// This is the check that getOrgPlanLimits uses to skip all enforcement.
	// When this returns false, all plan gates return nil (allowed).
}

// Addon with EffectiveLimits integration.

func TestAddon_EffectiveLimits_Integration(t *testing.T) {
	t.Parallel()

	// Pro org with 2 concurrency_100 packs should get base + 200.
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 200
	if result.MaxConcurrentRuns != want {
		t.Errorf("MaxConcurrentRuns = %d, want %d (base + 2x100 packs)", result.MaxConcurrentRuns, want)
	}

	// Deactivate addon -> back to base.
	deactivated := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 2, Active: false},
	}
	result = EffectiveLimits(base, deactivated)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("after deactivation MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
}
