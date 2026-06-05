package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(
		err, &le))
	assert.Equal(t, "grace_period_expired",

		le.Code)

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
	require.NoError(t,
		err)

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
	require.Error(t,
		err)

	var le *LimitError
	require.True(t, isLimitError(
		err, &le))
	assert.Equal(t, "payment_restricted",

		le.
			Code)

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
	require.NoError(t,
		store.CreateAddon(context.
			Background(), addon))
	require.NoError(t,
		store.DeactivateAddon(context.
			Background(), "addon-1"))
	require.NoError(t,
		store.DeactivateAddon(context.
			Background(), "addon-1"))

	// Deactivate once.

	// Deactivate again -- should be idempotent, no error.

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
		assert.True(t, reg.
			AllowsFeature(domain.
				PlanEnterprise,

				f))

	}

	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, reg.
			AllowsFeature(domain.
				PlanEnterprise,

				f))

	}

	enterpriseLimits := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1, enterpriseLimits.
		MaxConcurrentRuns,
	)
	assert.EqualValues(t, -1, enterpriseLimits.
		MaxWorkflowDAGSteps,
	)
	assert.EqualValues(t, -1, enterpriseLimits.
		MaxScheduledJobs,
	)

	// Verify unlimited limits.

}

func TestSelfHosted_EditionCommunity_SkipsGating(t *testing.T) {
	t.Parallel()

	// EditionCommunity should not require HTTP mode gating.
	edition := domain.EditionCommunity
	assert.False(t, edition.
		RequiresHTTPModeGating(),
	)

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
	assert.Equal(t, want,
		result.
			MaxConcurrentRuns,
	)

	// Deactivate addon -> back to base.
	deactivated := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 2, Active: false},
	}
	result = EffectiveLimits(base, deactivated)
	assert.Equal(t, base.
		MaxConcurrentRuns,

		result.MaxConcurrentRuns,
	)

}
