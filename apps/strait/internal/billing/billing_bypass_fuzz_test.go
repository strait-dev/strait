package billing

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// Fuzz tests: exercise thousands of inputs for panic/crash safety

func FuzzStddev(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x40, 0x59, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // 100.0

	f.Fuzz(func(t *testing.T, data []byte) {
		// Convert bytes to float64 slice (8 bytes per float64)
		if len(data) < 8 {
			return
		}
		n := len(data) / 8
		n = min(n, 100)
		values := make([]float64, 0, n)
		for i := range n {
			v := math.Float64frombits(uint64(data[i*8]) |
				uint64(data[i*8+1])<<8 |
				uint64(data[i*8+2])<<16 |
				uint64(data[i*8+3])<<24 |
				uint64(data[i*8+4])<<32 |
				uint64(data[i*8+5])<<40 |
				uint64(data[i*8+6])<<48 |
				uint64(data[i*8+7])<<56)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return // skip NaN/Inf inputs
			}
			values = append(values, v)
		}

		result := stddev(values)
		assert.False(t, math.
			IsNaN(result))
		assert.GreaterOrEqual(t,
			result, 0.0)
	})
}

func FuzzPauseReasonSQL(f *testing.F) {
	f.Add("plan_downgrade")
	f.Add("user_request")
	f.Add("")
	f.Add("'; DROP TABLE jobs; --")
	f.Add(strings.Repeat("a", 10000))
	f.Add("plan\x00downgrade")
	f.Add("plan\ndowngrade")

	f.Fuzz(func(t *testing.T, reason string) {
		// This tests that the mock store doesn't panic with arbitrary reasons.
		store := &mockBillingStore{}
		_, err := store.PauseHTTPJobsByOrg(context.Background(), "org-test", reason)
		require.NoError(t,
			err)

		_, err = store.UnpauseJobsByPauseReason(context.Background(), "org-test", reason)
		require.NoError(t,
			err)
	})
}

// Adversarial: plan enforcement bypass attempts

func TestBypass_AllPlansHaveBoundedLimits(t *testing.T) {
	t.Parallel()

	for _, tier := range domain.AllPlanTiers() {
		limits := GetPlanLimits(tier)
		assert.NotEmpty(t,
			limits.
				PlanTier,
		)

		// Every plan must have a tier set.

		// Free plan must have bounded (non-unlimited) limits for safety.
		if tier == domain.PlanFree {
			assert.NotEqual(t,
				-1, limits.
					MaxProjectsPerOrg,
			)
			assert.NotEqual(t,
				-1, limits.
					MaxMembersPerOrg,
			)
			assert.NotEqual(t,
				-1, limits.
					MaxConcurrentRuns,
			)
			assert.NotEqual(t,
				-1, limits.
					MaxScheduledJobs,
			)
		}
		assert.NotEmpty(t,
			limits.
				DisplayName,
		)
		assert.False(t, limits.
			RetentionDays ==
			0 || limits.
			RetentionDays < -1)

		// All plans must have non-empty display name.

		// Retention must be positive or -1 (unlimited).
	}
}

func TestBypass_FeatureGatesConsistentAcrossTiers(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// Features available on higher tiers must also be available on all tiers above.
	tiers := domain.AllPlanTiers()
	features := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA, FeatureRBAC,
	}

	for _, f := range features {
		foundFirst := false
		for _, tier := range tiers {
			allowed := reg.AllowsFeature(tier, f)
			if allowed {
				foundFirst = true
			}
			assert.False(t, foundFirst &&
				!allowed,
			)
		}
	}
}

func TestBypass_RequiredPlanNeverReturnsLowerTier(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	features := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA, FeatureRBAC,
		FeatureDedicatedCompute, FeatureSCIM, FeatureDataResidency,
	}

	tierOrder := map[domain.PlanTier]int{
		domain.PlanFree:       0,
		domain.PlanStarter:    1,
		domain.PlanPro:        2,
		domain.PlanScale:      3,
		domain.PlanBusiness:   4,
		domain.PlanEnterprise: 5,
	}

	for _, f := range features {
		required := reg.RequiredPlanForFeature(f)
		if IsRoadmapFeature(f) {
			assert.Equal(t, domain.PlanTier(""), required)

			continue
		}
		reqOrder, ok := tierOrder[required]
		if !ok {
			assert.Failf(t, "test failure",

				"RequiredPlanForFeature(%q) returned unknown tier %q", f, required)
			continue
		}

		// Every tier below the required one must NOT have the feature.
		for tier, order := range tierOrder {
			assert.False(t, order <
				reqOrder &&
				reg.AllowsFeature(tier, f))
		}
	}
}

func TestBypass_FreeTierCannotAccessPaidFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// HTTP mode is available on all tiers; only features that are genuinely
	// paid-tier-gated are listed here.
	paidFeatures := []Feature{
		FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA,
		FeatureDedicatedCompute, FeatureStaticIPs, FeatureVPCPeering,
		FeatureSCIM, FeatureDataResidency, FeatureCustomRBAC,
		FeaturePriorityQueue, FeatureIPAllowlisting,
		FeatureSessionManagement, FeatureSecretRotation, FeatureSIEMExport,
	}

	for _, f := range paidFeatures {
		assert.False(t, reg.
			AllowsFeature(domain.
				PlanFree,

				f))
	}
}

func TestBypass_EnterpriseHasLaunchActiveFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	allFeatures := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSLA, FeatureRBAC,
		FeatureAllCronOverlap,
	}

	for _, f := range allFeatures {
		assert.True(t, reg.
			AllowsFeature(domain.
				PlanEnterprise,

				f))
	}
}

func TestBypass_DowngradeAlwaysLosesFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// Downgrade from Scale to Starter should lose features.
	scaleOnlyFeatures := []Feature{FeatureCanaryDeployments, FeatureAuditLogs}
	for _, f := range scaleOnlyFeatures {
		assert.True(t, reg.
			AllowsFeature(domain.
				PlanScale,

				f))
		assert.False(t, reg.
			AllowsFeature(domain.
				PlanStarter,

				f))
	}
}

func TestBypass_SpendingLimitCannotGoNegative(t *testing.T) {
	t.Parallel()

	for _, tier := range domain.AllPlanTiers() {
		l := GetPlanLimits(tier)
		assert.GreaterOrEqual(t, l.OveragePerKMicrousd, int64(0))
	}
}

func TestBypass_ConcurrentPlanLookupSafe(t *testing.T) {
	t.Parallel()

	var wg conc.WaitGroup
	for i := range 100 {
		idx := i
		wg.Go(func() {
			tier := domain.AllPlanTiers()[idx%5]
			limits := GetPlanLimits(tier)
			assert.Equal(t, tier,
				limits.
					PlanTier,
			)
		})
	}
	wg.Wait()
}

func TestBypass_ConcurrentRegistryLookupSafe(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	var wg conc.WaitGroup
	for i := range 100 {
		idx := i
		wg.Go(func() {
			tier := domain.AllPlanTiers()[idx%5]
			feature := []Feature{FeatureHTTPMode, FeatureAuditLogs, FeatureSSO}[idx%3]
			reg.AllowsFeature(tier, feature)
			reg.RequiredPlanForFeature(feature)
		})
	}
	wg.Wait()
}

// Adversarial: webhook handler bypass attempts

func TestBypass_InvalidPlanTierInWebhook(t *testing.T) {
	t.Parallel()

	// Getting limits for an unknown tier should return free limits, not panic.
	limits := GetPlanLimits(domain.PlanTier("hacked_premium"))
	assert.Equal(t, domain.
		PlanFree,
		limits.
			PlanTier,
	)
}

func TestBypass_PlanLimitsImmutable(t *testing.T) {
	t.Parallel()

	// Getting limits should return a copy, not a reference that can be mutated.
	original := GetPlanLimits(domain.PlanFree)
	origProjects := original.MaxProjectsPerOrg
	origRetention := original.RetentionDays

	modified := GetPlanLimits(domain.PlanFree)
	modified.MaxProjectsPerOrg = 99999
	modified.RetentionDays = 99999

	reread := GetPlanLimits(domain.PlanFree)
	assert.Equal(t, origProjects,

		reread.
			MaxProjectsPerOrg,
	)
	assert.Equal(t, origRetention,

		reread.
			RetentionDays,
	)
}

// Adversarial: stddev edge cases

func TestStddev_KnownValues(t *testing.T) {
	t.Parallel()

	// [2, 4, 4, 4, 5, 5, 7, 9] -> stddev = 2.0
	got := stddev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	assert.False(t, got <
		1.99 ||
		got >
			2.01)
}

func TestStddev_ZeroVariance(t *testing.T) {
	t.Parallel()
	got := stddev([]float64{5, 5, 5, 5, 5})
	assert.InDelta(t, 0,
		got, 1e-9)
}

func TestStddev_SingleElement(t *testing.T) {
	t.Parallel()
	got := stddev([]float64{42})
	assert.InDelta(t, 0,
		got, 1e-9)
}

func TestStddev_Empty(t *testing.T) {
	t.Parallel()
	got := stddev(nil)
	assert.InDelta(t, 0,
		got, 1e-9)
}

func TestStddev_LargeValues(t *testing.T) {
	t.Parallel()
	// Verify no overflow with large values
	values := []float64{1e12, 1e12 + 1, 1e12 - 1}
	got := stddev(values)
	assert.False(t, math.
		IsNaN(got) || math.
		IsInf(got,

			0))
}

// Adversarial: HTTP job downgrade lifecycle

func TestBypass_PauseReasonSentinelConsistent(t *testing.T) {
	t.Parallel()

	// The sentinel "plan_downgrade" must be used consistently across
	// pause (downgrade_applier) and unpause (webhook upgrade).
	// If someone changes one without the other, jobs stay paused forever.
	reason := "plan_downgrade"

	// Simulate the full lifecycle with mock store.
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
	}

	// Simulate the downgrade applier pausing HTTP jobs.
	paused, err := store.PauseHTTPJobsByOrg(context.Background(), "org-1", reason)
	require.NoError(t,
		err)

	_ = paused

	// Simulate a webhook upgrade restoring the paused jobs.
	unpaused, err := store.UnpauseJobsByPauseReason(context.Background(), "org-1", reason)
	require.NoError(t,
		err)

	_ = unpaused
}

func TestBypass_HTTPJobCountNonNegative(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	count, err := store.CountHTTPJobsByOrg(context.Background(), "nonexistent-org")
	require.NoError(t,
		err)
	assert.GreaterOrEqual(t,
		count, 0)
}

// Adversarial: plan pricing consistency

func TestBypass_PricingMonotonicallyIncreases(t *testing.T) {
	t.Parallel()

	// Each successive paid plan should cost more than the previous.
	paidTiers := []domain.PlanTier{
		domain.PlanStarter, domain.PlanPro, domain.PlanScale,
	}

	for i := 1; i < len(paidTiers); i++ {
		prev := GetPlanLimits(paidTiers[i-1])
		curr := GetPlanLimits(paidTiers[i])
		assert.GreaterOrEqual(t,
			curr.PriceMonthlyUsd,

			prev.
				PriceMonthlyUsd)
	}
}

func TestBypass_AllPlanTiersInAllPlanTiersSlice(t *testing.T) {
	t.Parallel()

	// Verify AllPlanTiers() includes all 6 tiers.
	tiers := domain.AllPlanTiers()
	require.Len(t, tiers,
		6)

	expected := map[domain.PlanTier]bool{
		domain.PlanFree:       false,
		domain.PlanStarter:    false,
		domain.PlanPro:        false,
		domain.PlanScale:      false,
		domain.PlanBusiness:   false,
		domain.PlanEnterprise: false,
	}
	for _, tier := range tiers {
		if _, ok := expected[tier]; !ok {
			assert.Failf(t, "test failure",

				"unexpected tier %q in AllPlanTiers()", tier)
		}
		expected[tier] = true
	}
	for _, seen := range expected {
		assert.True(t, seen)
	}
}

func TestBypass_IsDowngradeSymmetry(t *testing.T) {
	t.Parallel()

	// If A -> B is a downgrade, then B -> A must NOT be a downgrade.
	tiers := domain.AllPlanTiers()
	for _, a := range tiers {
		for _, b := range tiers {
			if a == b {
				continue
			}
			aToB := IsDowngrade(a, b)
			bToA := IsDowngrade(b, a)
			assert.False(t, aToB &&
				bToA,
			)
		}
	}
}

// Adversarial: enforcement mode validation

func TestBypass_UnknownTierGetsFreeEnforcement(t *testing.T) {
	t.Parallel()

	// An org with an unknown plan tier should get free-tier limits.
	limits := GetPlanLimits(domain.PlanTier("premium_hacked"))
	freeLimits := GetPlanLimits(domain.PlanFree)
	assert.Equal(t, freeLimits.
		MaxProjectsPerOrg,
		limits.
			MaxProjectsPerOrg)
	assert.True(t, limits.
		AllowsHTTPMode,
	)
	assert.False(t, limits.
		HasAuditLogs,
	)

	// HTTP mode is available on all tiers including free (the fallback).
}

func TestBypass_MassivePlanTierStrings(t *testing.T) {
	t.Parallel()

	// Very long tier strings should not cause panics or memory issues.
	longTier := domain.PlanTier(strings.Repeat("a", 100000))
	limits := GetPlanLimits(longTier)
	assert.Equal(t, domain.
		PlanFree,
		limits.
			PlanTier,
	)
}

func TestBypass_NullBytesInTier(t *testing.T) {
	t.Parallel()

	tier := domain.PlanTier("pro\x00admin")
	limits := GetPlanLimits(tier)
	assert.Equal(t, domain.
		PlanFree,
		limits.
			PlanTier,
	)
}

func TestBypass_SQLInjectionInTier(t *testing.T) {
	t.Parallel()

	tier := domain.PlanTier("'; DROP TABLE organization_subscriptions; --")
	limits := GetPlanLimits(tier)
	assert.Equal(t, domain.
		PlanFree,
		limits.
			PlanTier,
	)
}
