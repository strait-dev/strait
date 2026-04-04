package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestGetPlanLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tier           domain.PlanTier
		wantDisplay    string
		wantMonthly    int
		wantConcurrent int
		wantProjects   int
		wantMembers    int
		wantCreditCard bool
		wantRetention  int
	}{
		{
			name:           "free",
			tier:           domain.PlanFree,
			wantDisplay:    "Free",
			wantMonthly:    0,
			wantConcurrent: 5,
			wantProjects:   1,
			wantMembers:    1,
			wantCreditCard: false,
			wantRetention:  1,
		},
		{
			name:           "starter",
			tier:           domain.PlanStarter,
			wantDisplay:    "Starter",
			wantMonthly:    1999,
			wantConcurrent: 25,
			wantProjects:   3,
			wantMembers:    5,
			wantCreditCard: true,
			wantRetention:  7,
		},
		{
			name:           "pro",
			tier:           domain.PlanPro,
			wantDisplay:    "Pro",
			wantMonthly:    4999,
			wantConcurrent: 100,
			wantProjects:   10,
			wantMembers:    10,
			wantCreditCard: true,
			wantRetention:  30,
		},
		{
			name:           "scale",
			tier:           domain.PlanScale,
			wantDisplay:    "Scale",
			wantMonthly:    9900,
			wantConcurrent: 500,
			wantProjects:   50,
			wantMembers:    50,
			wantCreditCard: true,
			wantRetention:  60,
		},
		{
			name:           "enterprise",
			tier:           domain.PlanEnterprise,
			wantDisplay:    "Enterprise",
			wantMonthly:    0,
			wantConcurrent: -1,
			wantProjects:   -1,
			wantMembers:    -1,
			wantCreditCard: false,
			wantRetention:  90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)

			if limits.DisplayName != tt.wantDisplay {
				t.Errorf("DisplayName = %q, want %q", limits.DisplayName, tt.wantDisplay)
			}
			if limits.PriceMonthlyUsd != tt.wantMonthly {
				t.Errorf("PriceMonthlyUsd = %d, want %d", limits.PriceMonthlyUsd, tt.wantMonthly)
			}
			if limits.MaxConcurrentRuns != tt.wantConcurrent {
				t.Errorf("MaxConcurrentRuns = %d, want %d", limits.MaxConcurrentRuns, tt.wantConcurrent)
			}
			if limits.MaxProjectsPerOrg != tt.wantProjects {
				t.Errorf("MaxProjectsPerOrg = %d, want %d", limits.MaxProjectsPerOrg, tt.wantProjects)
			}
			if limits.MaxMembersPerOrg != tt.wantMembers {
				t.Errorf("MaxMembersPerOrg = %d, want %d", limits.MaxMembersPerOrg, tt.wantMembers)
			}
			if limits.RequiresCreditCard != tt.wantCreditCard {
				t.Errorf("RequiresCreditCard = %v, want %v", limits.RequiresCreditCard, tt.wantCreditCard)
			}
			if limits.RetentionDays != tt.wantRetention {
				t.Errorf("RetentionDays = %d, want %d", limits.RetentionDays, tt.wantRetention)
			}
		})
	}
}

func TestGetPlanLimits_UnknownTier(t *testing.T) {
	t.Parallel()
	limits := GetPlanLimits(domain.PlanTier("unknown"))
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("expected fallback to free, got %q", limits.PlanTier)
	}
}

func TestFreeTierLimits(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	if free.FreeManagedRunsPerMonth != 0 {
		t.Errorf("FreeManagedRunsPerMonth = %d, want 0", free.FreeManagedRunsPerMonth)
	}
	if free.FreeManagedPreset != "micro" {
		t.Errorf("FreeManagedPreset = %q, want micro", free.FreeManagedPreset)
	}
	if free.FreeManagedMaxTimeout != 10 {
		t.Errorf("FreeManagedMaxTimeout = %d, want 10", free.FreeManagedMaxTimeout)
	}
	if free.ComputeCreditMicrousd != CreditFreeMicrousd {
		t.Errorf("ComputeCreditMicrousd = %d, want CreditFreeMicrousd (%d)", free.ComputeCreditMicrousd, CreditFreeMicrousd)
	}
}

func TestPaidTierCredits(t *testing.T) {
	t.Parallel()

	starter := GetPlanLimits(domain.PlanStarter)
	if starter.ComputeCreditMicrousd != 19_990_000 {
		t.Errorf("Starter credit = %d, want 19990000", starter.ComputeCreditMicrousd)
	}

	pro := GetPlanLimits(domain.PlanPro)
	if pro.ComputeCreditMicrousd != 49_990_000 {
		t.Errorf("Pro credit = %d, want 49990000", pro.ComputeCreditMicrousd)
	}

	scale := GetPlanLimits(domain.PlanScale)
	if scale.ComputeCreditMicrousd != 99_000_000 {
		t.Errorf("Scale credit = %d, want 99000000", scale.ComputeCreditMicrousd)
	}
}

func TestIsDowngrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from domain.PlanTier
		to   domain.PlanTier
		want bool
	}{
		// Downgrades.
		{"pro_to_starter", domain.PlanPro, domain.PlanStarter, true},
		{"pro_to_free", domain.PlanPro, domain.PlanFree, true},
		{"starter_to_free", domain.PlanStarter, domain.PlanFree, true},
		{"enterprise_to_pro", domain.PlanEnterprise, domain.PlanPro, true},
		{"enterprise_to_free", domain.PlanEnterprise, domain.PlanFree, true},
		{"scale_to_pro", domain.PlanScale, domain.PlanPro, true},
		{"scale_to_starter", domain.PlanScale, domain.PlanStarter, true},
		{"scale_to_free", domain.PlanScale, domain.PlanFree, true},
		{"enterprise_to_scale", domain.PlanEnterprise, domain.PlanScale, true},
		// Upgrades.
		{"starter_to_pro", domain.PlanStarter, domain.PlanPro, false},
		{"free_to_starter", domain.PlanFree, domain.PlanStarter, false},
		{"free_to_pro", domain.PlanFree, domain.PlanPro, false},
		{"free_to_scale", domain.PlanFree, domain.PlanScale, false},
		{"free_to_enterprise", domain.PlanFree, domain.PlanEnterprise, false},
		{"starter_to_scale", domain.PlanStarter, domain.PlanScale, false},
		{"pro_to_scale", domain.PlanPro, domain.PlanScale, false},
		{"scale_to_enterprise", domain.PlanScale, domain.PlanEnterprise, false},
		// Same tier.
		{"same_free", domain.PlanFree, domain.PlanFree, false},
		{"same_pro", domain.PlanPro, domain.PlanPro, false},
		{"same_scale", domain.PlanScale, domain.PlanScale, false},
		{"same_enterprise", domain.PlanEnterprise, domain.PlanEnterprise, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsDowngrade(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsDowngrade(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestPlanLimits_AllowsHTTPMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tier domain.PlanTier
		want bool
	}{
		{"free", domain.PlanFree, false},
		{"starter", domain.PlanStarter, false},
		{"pro", domain.PlanPro, true},
		{"scale", domain.PlanScale, true},
		{"enterprise", domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)
			if limits.AllowsHTTPMode != tt.want {
				t.Errorf("AllowsHTTPMode = %v, want %v", limits.AllowsHTTPMode, tt.want)
			}
		})
	}
}

func TestPlanLimits_DailyRunsUnlimited(t *testing.T) {
	t.Parallel()
	for _, tier := range domain.AllPlanTiers() {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tier)
			if limits.MaxRunsPerDay != -1 {
				t.Errorf("MaxRunsPerDay = %d, want -1 (unlimited)", limits.MaxRunsPerDay)
			}
		})
	}
}

func TestHTTPCostPerRunMicrousd(t *testing.T) {
	t.Parallel()
	if HTTPCostPerRunMicrousd != 20 {
		t.Errorf("HTTPCostPerRunMicrousd = %d, want 20", HTTPCostPerRunMicrousd)
	}
}

func TestAllPlansHaveEntries(t *testing.T) {
	t.Parallel()
	for _, tier := range domain.AllPlanTiers() {
		if _, ok := Plans[tier]; !ok {
			t.Errorf("missing plan definition for tier %q", tier)
		}
	}
	if len(Plans) != 5 {
		t.Errorf("expected 5 plan entries, got %d", len(Plans))
	}
}

func TestPlanLimits_WorkflowFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier              domain.PlanTier
		wantDAGSteps      int
		wantApprovalGates bool
		wantSubWorkflows  bool
		wantJobChaining   bool
		wantChainDepth    int
		wantCompensation  bool
		wantCanary        bool
	}{
		{domain.PlanFree, 10, false, false, false, 0, false, false},
		{domain.PlanStarter, 50, false, false, false, 0, false, false},
		{domain.PlanPro, 250, true, true, true, 10, true, false},
		{domain.PlanScale, 1000, true, true, true, 10, true, true},
		{domain.PlanEnterprise, -1, true, true, true, -1, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			l := GetPlanLimits(tt.tier)
			if l.MaxWorkflowDAGSteps != tt.wantDAGSteps {
				t.Errorf("MaxWorkflowDAGSteps = %d, want %d", l.MaxWorkflowDAGSteps, tt.wantDAGSteps)
			}
			if l.HasApprovalGates != tt.wantApprovalGates {
				t.Errorf("HasApprovalGates = %v, want %v", l.HasApprovalGates, tt.wantApprovalGates)
			}
			if l.HasSubWorkflows != tt.wantSubWorkflows {
				t.Errorf("HasSubWorkflows = %v, want %v", l.HasSubWorkflows, tt.wantSubWorkflows)
			}
			if l.HasJobChaining != tt.wantJobChaining {
				t.Errorf("HasJobChaining = %v, want %v", l.HasJobChaining, tt.wantJobChaining)
			}
			if l.MaxJobChainDepth != tt.wantChainDepth {
				t.Errorf("MaxJobChainDepth = %d, want %d", l.MaxJobChainDepth, tt.wantChainDepth)
			}
			if l.HasCompensatingTxns != tt.wantCompensation {
				t.Errorf("HasCompensatingTxns = %v, want %v", l.HasCompensatingTxns, tt.wantCompensation)
			}
			if l.HasCanaryDeployments != tt.wantCanary {
				t.Errorf("HasCanaryDeployments = %v, want %v", l.HasCanaryDeployments, tt.wantCanary)
			}
		})
	}
}

func TestPlanLimits_ResourceLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier               domain.PlanTier
		wantScheduled      int
		wantOverlapAll     bool
		wantEnvironments   int
		wantWebhookEP      int
		wantWebhookLevel   string
		wantAPIRate        int
		wantPresetRestrict bool // true if AllowedPresets is non-nil
	}{
		{domain.PlanFree, 10, false, 1, 0, "none", 60, true},
		{domain.PlanStarter, 25, true, 3, 3, "basic", 300, false},
		{domain.PlanPro, 100, true, 3, 10, "all", 1000, false},
		{domain.PlanScale, 500, true, 3, 25, "all", 3000, false},
		{domain.PlanEnterprise, -1, true, 3, -1, "all_custom", -1, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			l := GetPlanLimits(tt.tier)
			if l.MaxScheduledJobs != tt.wantScheduled {
				t.Errorf("MaxScheduledJobs = %d, want %d", l.MaxScheduledJobs, tt.wantScheduled)
			}
			if l.AllCronOverlapPolicies != tt.wantOverlapAll {
				t.Errorf("AllCronOverlapPolicies = %v, want %v", l.AllCronOverlapPolicies, tt.wantOverlapAll)
			}
			if l.MaxEnvironments != tt.wantEnvironments {
				t.Errorf("MaxEnvironments = %d, want %d", l.MaxEnvironments, tt.wantEnvironments)
			}
			if l.MaxWebhookEndpoints != tt.wantWebhookEP {
				t.Errorf("MaxWebhookEndpoints = %d, want %d", l.MaxWebhookEndpoints, tt.wantWebhookEP)
			}
			if l.WebhookEventLevel != tt.wantWebhookLevel {
				t.Errorf("WebhookEventLevel = %q, want %q", l.WebhookEventLevel, tt.wantWebhookLevel)
			}
			if l.APIRateLimit != tt.wantAPIRate {
				t.Errorf("APIRateLimit = %d, want %d", l.APIRateLimit, tt.wantAPIRate)
			}
			hasRestrict := l.AllowedPresets != nil
			if hasRestrict != tt.wantPresetRestrict {
				t.Errorf("AllowedPresets restricted = %v, want %v", hasRestrict, tt.wantPresetRestrict)
			}
		})
	}
}

func TestPlanLimits_IsPresetAllowed(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	// Free allows micro through medium-2x.
	for _, p := range []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x"} {
		if !free.IsPresetAllowed(p) {
			t.Errorf("Free.IsPresetAllowed(%q) = false, want true", p)
		}
	}
	// Free blocks large presets.
	for _, p := range []string{"large-1x", "large-2x"} {
		if free.IsPresetAllowed(p) {
			t.Errorf("Free.IsPresetAllowed(%q) = true, want false", p)
		}
	}

	// Paid plans allow all presets.
	for _, tier := range []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanEnterprise} {
		limits := GetPlanLimits(tier)
		for _, p := range []string{"micro", "small-1x", "large-1x", "large-2x"} {
			if !limits.IsPresetAllowed(p) {
				t.Errorf("%s.IsPresetAllowed(%q) = false, want true", tier, p)
			}
		}
	}
}

func TestPlanLimits_AuditLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier domain.PlanTier
		want bool
	}{
		{domain.PlanFree, false},
		{domain.PlanStarter, false},
		{domain.PlanPro, false},
		{domain.PlanScale, true},
		{domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)
			if limits.HasAuditLogs != tt.want {
				t.Errorf("HasAuditLogs = %v, want %v", limits.HasAuditLogs, tt.want)
			}
		})
	}
}

func TestPlanLimits_ScaleConstants(t *testing.T) {
	t.Parallel()

	scale := GetPlanLimits(domain.PlanScale)
	if scale.PriceMonthlyUsd != PriceScaleMonthlyCents {
		t.Errorf("PriceMonthlyUsd = %d, want %d", scale.PriceMonthlyUsd, PriceScaleMonthlyCents)
	}
	if scale.PriceAnnualUsd != PriceScaleAnnualCents {
		t.Errorf("PriceAnnualUsd = %d, want %d", scale.PriceAnnualUsd, PriceScaleAnnualCents)
	}
	if scale.ComputeCreditMicrousd != CreditScaleMicrousd {
		t.Errorf("ComputeCreditMicrousd = %d, want %d", scale.ComputeCreditMicrousd, CreditScaleMicrousd)
	}
	if scale.MaxConcurrentRuns != ConcurrentScale {
		t.Errorf("MaxConcurrentRuns = %d, want %d", scale.MaxConcurrentRuns, ConcurrentScale)
	}
	if scale.OveragePerKRunsMicrousd != DefaultOveragePerKRunsMicrousd {
		t.Errorf("OveragePerKRunsMicrousd = %d, want %d", scale.OveragePerKRunsMicrousd, DefaultOveragePerKRunsMicrousd)
	}
}

func FuzzGetPlanLimits_NoPanic(f *testing.F) {
	f.Add("free")
	f.Add("starter")
	f.Add("pro")
	f.Add("scale")
	f.Add("enterprise")
	f.Add("")
	f.Add("unknown")
	f.Add("FREE")
	f.Add("pro\x00")

	f.Fuzz(func(t *testing.T, tier string) {
		// Should never panic, always returns valid limits.
		limits := GetPlanLimits(domain.PlanTier(tier))
		if limits.PlanTier == "" {
			t.Error("GetPlanLimits returned empty PlanTier")
		}
	})
}
