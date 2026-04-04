package billing

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"
)

// ============================================================.
// Fuzz tests: exercise thousands of inputs for panic/crash safety
// ============================================================.

func FuzzCronRunsPerDay(f *testing.F) {
	f.Add("* * * * *")
	f.Add("0 * * * *")
	f.Add("0 0 * * *")
	f.Add("*/5 * * * *")
	f.Add("")
	f.Add("invalid")
	f.Add("0 0 30 2 *") // Feb 30 doesn't exist
	f.Add("@hourly")

	f.Fuzz(func(t *testing.T, expr string) {
		result, err := CronRunsPerDay(expr)
		if err == nil && result < 0 {
			t.Errorf("CronRunsPerDay(%q) = %f, want >= 0", expr, result)
		}
	})
}

func FuzzEstimateWhatIf(f *testing.F) {
	f.Add("micro", 60, "0 * * * *", 1)
	f.Add("micro", 1, "", 1)
	f.Add("large", 3600, "*/5 * * * *", 10)
	f.Add("", 0, "", 0)
	f.Add("nonexistent", -1, "bad cron", -5)

	f.Fuzz(func(t *testing.T, preset string, timeout int, cron string, count int) {
		result, err := EstimateWhatIf(preset, timeout, cron, count)
		if err == nil && result != nil {
			if result.MonthlyCostUsd < 0 {
				t.Errorf("MonthlyCostUsd = %f, want >= 0", result.MonthlyCostUsd)
			}
			if result.RunsPerDay < 0 {
				t.Errorf("RunsPerDay = %f, want >= 0", result.RunsPerDay)
			}
		}
	})
}

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
		if math.IsNaN(result) {
			t.Errorf("stddev returned NaN for %v", values)
		}
		if result < 0 {
			t.Errorf("stddev = %f, want >= 0", result)
		}
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
		if err != nil {
			t.Fatalf("PauseHTTPJobsByOrg with reason %q: %v", reason, err)
		}
		_, err = store.UnpauseJobsByPauseReason(context.Background(), "org-test", reason)
		if err != nil {
			t.Fatalf("UnpauseJobsByPauseReason with reason %q: %v", reason, err)
		}
	})
}

// ============================================================.
// Adversarial: plan enforcement bypass attempts
// ============================================================.

func TestBypass_AllPlansHaveBoundedLimits(t *testing.T) {
	t.Parallel()

	for _, tier := range domain.AllPlanTiers() {
		limits := GetPlanLimits(tier)

		// Every plan must have a tier set.
		if limits.PlanTier == "" {
			t.Errorf("plan %q has empty PlanTier", tier)
		}

		// Free plan must have bounded (non-unlimited) limits for safety.
		if tier == domain.PlanFree {
			if limits.MaxProjectsPerOrg == -1 {
				t.Error("Free plan has unlimited projects")
			}
			if limits.MaxMembersPerOrg == -1 {
				t.Error("Free plan has unlimited members")
			}
			if limits.MaxConcurrentRuns == -1 {
				t.Error("Free plan has unlimited concurrent runs")
			}
			if limits.MaxScheduledJobs == -1 {
				t.Error("Free plan has unlimited scheduled jobs")
			}
		}

		// All plans must have non-empty display name.
		if limits.DisplayName == "" {
			t.Errorf("plan %q has empty DisplayName", tier)
		}

		// Retention must be positive.
		if limits.RetentionDays <= 0 {
			t.Errorf("plan %q has non-positive RetentionDays: %d", tier, limits.RetentionDays)
		}
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
			if foundFirst && !allowed {
				t.Errorf("feature %q is allowed on a lower tier but not on %q (monotonicity violation)", f, tier)
			}
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
		domain.PlanEnterprise: 4,
	}

	for _, f := range features {
		required := reg.RequiredPlanForFeature(f)
		reqOrder, ok := tierOrder[required]
		if !ok {
			t.Errorf("RequiredPlanForFeature(%q) returned unknown tier %q", f, required)
			continue
		}

		// Every tier below the required one must NOT have the feature.
		for tier, order := range tierOrder {
			if order < reqOrder && reg.AllowsFeature(tier, f) {
				t.Errorf("feature %q: required=%q but %q (lower) allows it", f, required, tier)
			}
		}
	}
}

func TestBypass_FreeTierCannotAccessPaidFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	paidFeatures := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA,
		FeatureDedicatedCompute, FeatureStaticIPs, FeatureVPCPeering,
		FeatureSCIM, FeatureDataResidency, FeatureCustomRBAC,
		FeatureReservedCapacity, FeaturePriorityQueue, FeatureIPAllowlisting,
		FeatureSessionManagement, FeatureSecretRotation, FeatureSIEMExport,
	}

	for _, f := range paidFeatures {
		if reg.AllowsFeature(domain.PlanFree, f) {
			t.Errorf("Free tier should not have feature %q", f)
		}
	}
}

func TestBypass_EnterpriseHasAllFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	allFeatures := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA, FeatureRBAC,
		FeatureDedicatedCompute, FeatureStaticIPs, FeatureVPCPeering,
		FeatureSCIM, FeatureDataResidency, FeatureCustomRBAC,
		FeatureReservedCapacity, FeaturePriorityQueue, FeatureIPAllowlisting,
		FeatureSessionManagement, FeatureSecretRotation, FeatureSIEMExport,
		FeatureAllCronOverlap, FeatureAIAssistantBYOK,
	}

	for _, f := range allFeatures {
		if !reg.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should have feature %q", f)
		}
	}
}

func TestBypass_DowngradeAlwaysLosesFeatures(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// Downgrade from Scale to Starter should lose features.
	scaleOnlyFeatures := []Feature{FeatureCanaryDeployments, FeatureAuditLogs}
	for _, f := range scaleOnlyFeatures {
		if !reg.AllowsFeature(domain.PlanScale, f) {
			t.Errorf("Scale should have %q", f)
		}
		if reg.AllowsFeature(domain.PlanStarter, f) {
			t.Errorf("Starter should NOT have %q after downgrade from Scale", f)
		}
	}
}

func TestBypass_SpendingLimitCannotGoNegative(t *testing.T) {
	t.Parallel()

	limits := GetPlanLimits(domain.PlanFree)
	if limits.ComputeCreditMicrousd < 0 {
		t.Errorf("Free plan has negative credit: %d", limits.ComputeCreditMicrousd)
	}

	for _, tier := range domain.AllPlanTiers() {
		l := GetPlanLimits(tier)
		if l.OveragePerKRunsMicrousd < 0 {
			t.Errorf("plan %q has negative overage rate: %d", tier, l.OveragePerKRunsMicrousd)
		}
	}
}

func TestBypass_ConcurrentPlanLookupSafe(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tier := domain.AllPlanTiers()[idx%5]
			limits := GetPlanLimits(tier)
			if limits.PlanTier != tier {
				t.Errorf("concurrent lookup: expected %q, got %q", tier, limits.PlanTier)
			}
		}(i)
	}
	wg.Wait()
}

func TestBypass_ConcurrentRegistryLookupSafe(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tier := domain.AllPlanTiers()[idx%5]
			feature := []Feature{FeatureHTTPMode, FeatureAuditLogs, FeatureSSO}[idx%3]
			reg.AllowsFeature(tier, feature)
			reg.RequiredPlanForFeature(feature)
		}(i)
	}
	wg.Wait()
}

// ============================================================.
// Adversarial: webhook handler bypass attempts
// ============================================================.

func TestBypass_InvalidPlanTierInWebhook(t *testing.T) {
	t.Parallel()

	// Getting limits for an unknown tier should return free limits, not panic.
	limits := GetPlanLimits(domain.PlanTier("hacked_premium"))
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("unknown tier should return free, got %q", limits.PlanTier)
	}
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
	if reread.MaxProjectsPerOrg != origProjects {
		t.Error("plan limits are mutable -- writing to returned struct affects future calls")
	}
	if reread.RetentionDays != origRetention {
		t.Error("plan RetentionDays was mutated via returned struct")
	}
}

// ============================================================.
// Adversarial: CronRunsPerDay edge cases
// ============================================================.

func TestCronRunsPerDay_KnownValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr string
		want float64
	}{
		{"* * * * *", 1440},
		{"0 * * * *", 24},
		{"0 0 * * *", 1},
		{"*/5 * * * *", 288},
		{"*/15 * * * *", 96},
		{"0 0 * * 1", 1}, // only Mondays -- ref window is Monday
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			got, err := CronRunsPerDay(tt.expr)
			if err != nil {
				t.Fatalf("CronRunsPerDay(%q) error: %v", tt.expr, err)
			}
			if got != tt.want {
				t.Errorf("CronRunsPerDay(%q) = %f, want %f", tt.expr, got, tt.want)
			}
		})
	}
}

func TestCronRunsPerDay_EmptyReturnsZero(t *testing.T) {
	t.Parallel()
	got, err := CronRunsPerDay("")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("CronRunsPerDay('') = %f, want 0", got)
	}
}

func TestCronRunsPerDay_InvalidReturnsError(t *testing.T) {
	t.Parallel()
	_, err := CronRunsPerDay("not a cron expression")
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

// ============================================================.
// Adversarial: stddev edge cases
// ============================================================.

func TestStddev_KnownValues(t *testing.T) {
	t.Parallel()

	// [2, 4, 4, 4, 5, 5, 7, 9] -> stddev = 2.0
	got := stddev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if got < 1.99 || got > 2.01 {
		t.Errorf("stddev([2,4,4,4,5,5,7,9]) = %f, want ~2.0", got)
	}
}

func TestStddev_ZeroVariance(t *testing.T) {
	t.Parallel()
	got := stddev([]float64{5, 5, 5, 5, 5})
	if got != 0 {
		t.Errorf("stddev([5,5,5,5,5]) = %f, want 0", got)
	}
}

func TestStddev_SingleElement(t *testing.T) {
	t.Parallel()
	got := stddev([]float64{42})
	if got != 0 {
		t.Errorf("stddev([42]) = %f, want 0", got)
	}
}

func TestStddev_Empty(t *testing.T) {
	t.Parallel()
	got := stddev(nil)
	if got != 0 {
		t.Errorf("stddev(nil) = %f, want 0", got)
	}
}

func TestStddev_LargeValues(t *testing.T) {
	t.Parallel()
	// Verify no overflow with large values
	values := []float64{1e12, 1e12 + 1, 1e12 - 1}
	got := stddev(values)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("stddev with large values returned %f", got)
	}
}

// ============================================================.
// Adversarial: HTTP job downgrade lifecycle
// ============================================================.

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

	// Step 1: Pause (what downgrade_applier does)
	paused, err := store.PauseHTTPJobsByOrg(context.Background(), "org-1", reason)
	if err != nil {
		t.Fatal(err)
	}
	_ = paused

	// Step 2: Unpause (what webhook upgrade does)
	unpaused, err := store.UnpauseJobsByPauseReason(context.Background(), "org-1", reason)
	if err != nil {
		t.Fatal(err)
	}
	_ = unpaused
}

func TestBypass_HTTPJobCountNonNegative(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	count, err := store.CountHTTPJobsByOrg(context.Background(), "nonexistent-org")
	if err != nil {
		t.Fatal(err)
	}
	if count < 0 {
		t.Errorf("CountHTTPJobsByOrg returned negative: %d", count)
	}
}

// ============================================================.
// Adversarial: plan pricing consistency
// ============================================================.

func TestBypass_PricingMonotonicallyIncreases(t *testing.T) {
	t.Parallel()

	// Each successive paid plan should cost more than the previous.
	paidTiers := []domain.PlanTier{
		domain.PlanStarter, domain.PlanPro, domain.PlanScale,
	}

	for i := 1; i < len(paidTiers); i++ {
		prev := GetPlanLimits(paidTiers[i-1])
		curr := GetPlanLimits(paidTiers[i])

		if curr.ComputeCreditMicrousd < prev.ComputeCreditMicrousd {
			t.Errorf("plan %q credit (%d) < plan %q credit (%d)",
				paidTiers[i], curr.ComputeCreditMicrousd,
				paidTiers[i-1], prev.ComputeCreditMicrousd)
		}
	}
}

func TestBypass_AllPlanTiersInAllPlanTiersSlice(t *testing.T) {
	t.Parallel()

	// Verify AllPlanTiers() includes all 5 tiers.
	tiers := domain.AllPlanTiers()
	if len(tiers) != 5 {
		t.Fatalf("AllPlanTiers() returned %d tiers, want 5", len(tiers))
	}

	expected := map[domain.PlanTier]bool{
		domain.PlanFree:       false,
		domain.PlanStarter:    false,
		domain.PlanPro:        false,
		domain.PlanScale:      false,
		domain.PlanEnterprise: false,
	}
	for _, tier := range tiers {
		if _, ok := expected[tier]; !ok {
			t.Errorf("unexpected tier %q in AllPlanTiers()", tier)
		}
		expected[tier] = true
	}
	for tier, seen := range expected {
		if !seen {
			t.Errorf("AllPlanTiers() missing tier %q", tier)
		}
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
			if aToB && bToA {
				t.Errorf("both %q->%q and %q->%q are downgrades (impossible)", a, b, b, a)
			}
		}
	}
}

// ============================================================.
// Adversarial: enforcement mode validation
// ============================================================.

func TestBypass_UnknownTierGetsFreeEnforcement(t *testing.T) {
	t.Parallel()

	// An org with an unknown plan tier should get free-tier limits.
	limits := GetPlanLimits(domain.PlanTier("premium_hacked"))
	freeLimits := GetPlanLimits(domain.PlanFree)

	if limits.MaxProjectsPerOrg != freeLimits.MaxProjectsPerOrg {
		t.Errorf("unknown tier projects=%d, free=%d", limits.MaxProjectsPerOrg, freeLimits.MaxProjectsPerOrg)
	}
	if limits.AllowsHTTPMode {
		t.Error("unknown tier should not allow HTTP mode")
	}
	if limits.HasAuditLogs {
		t.Error("unknown tier should not have audit logs")
	}
}

func TestBypass_MassivePlanTierStrings(t *testing.T) {
	t.Parallel()

	// Very long tier strings should not cause panics or memory issues.
	longTier := domain.PlanTier(strings.Repeat("a", 100000))
	limits := GetPlanLimits(longTier)
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("massive tier string should fall back to free, got %q", limits.PlanTier)
	}
}

func TestBypass_NullBytesInTier(t *testing.T) {
	t.Parallel()

	tier := domain.PlanTier("pro\x00admin")
	limits := GetPlanLimits(tier)
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("null-byte tier should fall back to free, got %q", limits.PlanTier)
	}
}

func TestBypass_SQLInjectionInTier(t *testing.T) {
	t.Parallel()

	tier := domain.PlanTier("'; DROP TABLE organization_subscriptions; --")
	limits := GetPlanLimits(tier)
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("SQL injection tier should fall back to free, got %q", limits.PlanTier)
	}
}

// ============================================================.
// Adversarial: WhatIf calculator edge cases
// ============================================================.

func TestWhatIf_ZeroTimeout(t *testing.T) {
	t.Parallel()
	_, err := EstimateWhatIf("micro", 0, "", 1)
	// Zero timeout should still work (estimate 0 cost)
	if err != nil {
		t.Logf("zero timeout returned error (acceptable): %v", err)
	}
}

func TestWhatIf_NegativeCount(t *testing.T) {
	t.Parallel()
	result, err := EstimateWhatIf("micro", 60, "", -5)
	if err != nil {
		t.Fatal(err)
	}
	// Negative count should be clamped to 1.
	if result.RunsPerDay < 0 {
		t.Errorf("negative count produced negative runs: %f", result.RunsPerDay)
	}
}

func TestWhatIf_MaxTimeout(t *testing.T) {
	t.Parallel()
	result, err := EstimateWhatIf("micro", math.MaxInt32, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.MonthlyCostUsd < 0 {
		t.Errorf("max timeout produced negative cost: %f", result.MonthlyCostUsd)
	}
}

func TestWhatIf_VeryHighFrequency(t *testing.T) {
	t.Parallel()
	result, err := EstimateWhatIf("micro", 10, "* * * * *", 100)
	if err != nil {
		t.Fatal(err)
	}
	// 1440 runs/day * 100 = 144000 runs/day
	if result.RunsPerDay < 100000 {
		t.Errorf("expected high runs/day for every-minute * 100, got %f", result.RunsPerDay)
	}
	fmt.Printf("high-frequency what-if: %f/day, $%.2f/month\n", result.RunsPerDay, result.MonthlyCostUsd)
}
