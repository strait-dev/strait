package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestEffectiveLimits_NoAddons(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	result := EffectiveLimits(base, nil)

	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
	if result.MaxMembersPerOrg != base.MaxMembersPerOrg {
		t.Errorf("MaxMembersPerOrg = %d, want %d", result.MaxMembersPerOrg, base.MaxMembersPerOrg)
	}
	if result.RetentionDays != base.RetentionDays {
		t.Errorf("RetentionDays = %d, want %d", result.RetentionDays, base.RetentionDays)
	}
}

func TestEffectiveLimits_ConcurrentRunsPack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 50
	if result.MaxConcurrentRuns != want {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, want)
	}
}

func TestEffectiveLimits_MultiplePacksStack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: 3, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxConcurrentRuns + 150 // 3 packs x 50
	if result.MaxConcurrentRuns != want {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, want)
	}
}

func TestEffectiveLimits_MembersPack(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanStarter)
	addons := []Addon{
		{AddonType: AddonMembers, Quantity: 5, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxMembersPerOrg + 5 // 5 seats x 1
	if result.MaxMembersPerOrg != want {
		t.Errorf("MaxMembersPerOrg = %d, want %d", result.MaxMembersPerOrg, want)
	}
}

func TestEffectiveLimits_DataRetention_MaxCap90(t *testing.T) {
	t.Parallel()

	// Starter has 7 days. One pack adds 30 days = 37.
	starter := GetPlanLimits(domain.PlanStarter)
	result := EffectiveLimits(starter, []Addon{
		{AddonType: AddonDataRetention, Quantity: 1, Active: true},
	})
	if result.RetentionDays != 37 {
		t.Errorf("Starter + 1 retention pack = %d, want 37", result.RetentionDays)
	}

	// Three packs would be 7 + 90 = 97, capped at 90.
	result = EffectiveLimits(starter, []Addon{
		{AddonType: AddonDataRetention, Quantity: 3, Active: true},
	})
	if result.RetentionDays != 90 {
		t.Errorf("Starter + 3 retention packs = %d, want 90 (capped)", result.RetentionDays)
	}

	// Pro has 30 days. Two packs = 30 + 60 = 90.
	pro := GetPlanLimits(domain.PlanPro)
	result = EffectiveLimits(pro, []Addon{
		{AddonType: AddonDataRetention, Quantity: 2, Active: true},
	})
	if result.RetentionDays != 90 {
		t.Errorf("Pro + 2 retention packs = %d, want 90", result.RetentionDays)
	}
}

func TestEffectiveLimits_MixedAddons(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: 2, Active: true},
		{AddonType: AddonMembers, Quantity: 3, Active: true},
		{AddonType: AddonCronSchedules, Quantity: 1, Active: true},
	}

	result := EffectiveLimits(base, addons)

	if result.MaxConcurrentRuns != base.MaxConcurrentRuns+100 {
		t.Errorf("MaxConcurrentRuns = %d, want %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns+100)
	}
	if result.MaxMembersPerOrg != base.MaxMembersPerOrg+3 {
		t.Errorf("MaxMembersPerOrg = %d, want %d", result.MaxMembersPerOrg, base.MaxMembersPerOrg+3)
	}
	if result.MaxScheduledJobs != base.MaxScheduledJobs+25 {
		t.Errorf("MaxScheduledJobs = %d, want %d", result.MaxScheduledJobs, base.MaxScheduledJobs+25)
	}
}

func TestEffectiveLimits_NegativeQuantity_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: -5, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("negative quantity should be ignored: got %d, want %d",
			result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
}

func TestEffectiveLimits_UnknownAddonType_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonType("nonexistent"), Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns ||
		result.MaxMembersPerOrg != base.MaxMembersPerOrg ||
		result.RetentionDays != base.RetentionDays ||
		result.MaxScheduledJobs != base.MaxScheduledJobs ||
		result.MaxWebhookEndpoints != base.MaxWebhookEndpoints {
		t.Error("unknown addon type should have no effect on limits")
	}
}

func TestEffectiveLimits_InactiveAddons_NotApplied(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: 5, Active: false},
		{AddonType: AddonMembers, Quantity: 10, Active: false},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("inactive addon applied: MaxConcurrentRuns = %d, want %d",
			result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
	if result.MaxMembersPerOrg != base.MaxMembersPerOrg {
		t.Errorf("inactive addon applied: MaxMembersPerOrg = %d, want %d",
			result.MaxMembersPerOrg, base.MaxMembersPerOrg)
	}
}

func TestEffectiveLimits_UnlimitedNotModified(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanEnterprise)
	addons := []Addon{
		{AddonType: AddonConcurrentRuns, Quantity: 10, Active: true},
		{AddonType: AddonMembers, Quantity: 10, Active: true},
		{AddonType: AddonCronSchedules, Quantity: 10, Active: true},
		{AddonType: AddonWebhookEndpoints, Quantity: 10, Active: true},
	}

	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != -1 {
		t.Errorf("enterprise concurrent runs should stay unlimited, got %d", result.MaxConcurrentRuns)
	}
	if result.MaxMembersPerOrg != -1 {
		t.Errorf("enterprise members should stay unlimited, got %d", result.MaxMembersPerOrg)
	}
	if result.MaxScheduledJobs != -1 {
		t.Errorf("enterprise scheduled jobs should stay unlimited, got %d", result.MaxScheduledJobs)
	}
	if result.MaxWebhookEndpoints != -1 {
		t.Errorf("enterprise webhook endpoints should stay unlimited, got %d", result.MaxWebhookEndpoints)
	}
}

func TestEffectiveLimits_WebhookEndpoints(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro) // 10 endpoints
	addons := []Addon{
		{AddonType: AddonWebhookEndpoints, Quantity: 2, Active: true},
	}

	result := EffectiveLimits(base, addons)
	want := base.MaxWebhookEndpoints + 10 // 2 packs x 5
	if result.MaxWebhookEndpoints != want {
		t.Errorf("MaxWebhookEndpoints = %d, want %d", result.MaxWebhookEndpoints, want)
	}
}

func TestIsValidAddonType(t *testing.T) {
	t.Parallel()

	for _, at := range AllAddonTypes() {
		if !IsValidAddonType(at) {
			t.Errorf("IsValidAddonType(%q) = false, want true", at)
		}
	}

	if IsValidAddonType(AddonType("nonexistent")) {
		t.Error("IsValidAddonType(nonexistent) = true, want false")
	}
	if IsValidAddonType(AddonType("")) {
		t.Error("IsValidAddonType(\"\") = true, want false")
	}
}

func TestAllAddonTypes_Count(t *testing.T) {
	t.Parallel()
	types := AllAddonTypes()
	if len(types) != 5 {
		t.Errorf("AllAddonTypes() count = %d, want 5", len(types))
	}
}

func TestAddonPacks_AllDefined(t *testing.T) {
	t.Parallel()
	for _, at := range AllAddonTypes() {
		if _, ok := AddonPacks[at]; !ok {
			t.Errorf("missing AddonPacks entry for %q", at)
		}
	}
}

func TestAddonPacks_PositiveValues(t *testing.T) {
	t.Parallel()
	for at, pack := range AddonPacks {
		if pack.PackSize <= 0 {
			t.Errorf("AddonPacks[%q].PackSize = %d, want > 0", at, pack.PackSize)
		}
		if pack.PriceCents <= 0 {
			t.Errorf("AddonPacks[%q].PriceCents = %d, want > 0", at, pack.PriceCents)
		}
		if pack.Type != at {
			t.Errorf("AddonPacks[%q].Type = %q, want %q", at, pack.Type, at)
		}
		if pack.DisplayName == "" {
			t.Errorf("AddonPacks[%q].DisplayName is empty", at)
		}
	}
}

func TestAddonPacks_DataRetention_HasMaxTotal(t *testing.T) {
	t.Parallel()
	pack := AddonPacks[AddonDataRetention]
	if pack.MaxTotal <= 0 {
		t.Errorf("DataRetention MaxTotal = %d, want > 0 (capped)", pack.MaxTotal)
	}
	if pack.MaxTotal != 90 {
		t.Errorf("DataRetention MaxTotal = %d, want 90", pack.MaxTotal)
	}
}

func FuzzEffectiveLimits(f *testing.F) {
	f.Add("concurrent_runs", 1, true)
	f.Add("members", 5, true)
	f.Add("data_retention", 3, true)
	f.Add("nonexistent", 10, true)
	f.Add("", 0, false)
	f.Add("concurrent_runs", -1, true)

	f.Fuzz(func(t *testing.T, addonType string, quantity int, active bool) {
		base := GetPlanLimits(domain.PlanPro)
		addons := []Addon{
			{AddonType: AddonType(addonType), Quantity: quantity, Active: active},
		}
		// Should never panic.
		result := EffectiveLimits(base, addons)
		// Base limits should never decrease.
		if result.MaxConcurrentRuns < base.MaxConcurrentRuns && base.MaxConcurrentRuns != -1 {
			t.Errorf("MaxConcurrentRuns decreased: %d < %d", result.MaxConcurrentRuns, base.MaxConcurrentRuns)
		}
	})
}
