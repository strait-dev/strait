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

	if free.FreeManagedPreset != "micro" {
		t.Errorf("FreeManagedPreset = %q, want micro", free.FreeManagedPreset)
	}
	if free.FreeManagedMaxTimeout != 10 {
		t.Errorf("FreeManagedMaxTimeout = %d, want 10", free.FreeManagedMaxTimeout)
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
	if scale.MaxConcurrentRuns != ConcurrentScale {
		t.Errorf("MaxConcurrentRuns = %d, want %d", scale.MaxConcurrentRuns, ConcurrentScale)
	}
	if scale.OveragePerKRunsMicrousd != DefaultOveragePerKRunsMicrousd {
		t.Errorf("OveragePerKRunsMicrousd = %d, want %d", scale.OveragePerKRunsMicrousd, DefaultOveragePerKRunsMicrousd)
	}
}

func TestPlanConstants_Pricing(t *testing.T) {
	t.Parallel()
	if PriceStarterMonthlyCents != 1999 {
		t.Errorf("PriceStarterMonthlyCents = %d, want 1999", PriceStarterMonthlyCents)
	}
	if PriceStarterAnnualCents != 19999 {
		t.Errorf("PriceStarterAnnualCents = %d, want 19999", PriceStarterAnnualCents)
	}
	if PriceProMonthlyCents != 4999 {
		t.Errorf("PriceProMonthlyCents = %d, want 4999", PriceProMonthlyCents)
	}
	if PriceProAnnualCents != 49999 {
		t.Errorf("PriceProAnnualCents = %d, want 49999", PriceProAnnualCents)
	}
	if PriceScaleMonthlyCents != 9900 {
		t.Errorf("PriceScaleMonthlyCents = %d, want 9900", PriceScaleMonthlyCents)
	}
	if PriceScaleAnnualCents != 99000 {
		t.Errorf("PriceScaleAnnualCents = %d, want 99000", PriceScaleAnnualCents)
	}
}

func TestPlanConstants_Credits(t *testing.T) {
	t.Parallel()
	if CreditFreeMicrousd != 1_000_000 {
		t.Errorf("CreditFreeMicrousd = %d, want 1000000", CreditFreeMicrousd)
	}
	if CreditStarterMicrousd != 19_990_000 {
		t.Errorf("CreditStarterMicrousd = %d, want 19990000", CreditStarterMicrousd)
	}
	if CreditProMicrousd != 49_990_000 {
		t.Errorf("CreditProMicrousd = %d, want 49990000", CreditProMicrousd)
	}
	if CreditScaleMicrousd != 99_000_000 {
		t.Errorf("CreditScaleMicrousd = %d, want 99000000", CreditScaleMicrousd)
	}
}

func TestPlanConstants_Concurrent(t *testing.T) {
	t.Parallel()
	if ConcurrentFree != 5 {
		t.Errorf("ConcurrentFree = %d, want 5", ConcurrentFree)
	}
	if ConcurrentStarter != 25 {
		t.Errorf("ConcurrentStarter = %d, want 25", ConcurrentStarter)
	}
	if ConcurrentPro != 100 {
		t.Errorf("ConcurrentPro = %d, want 100", ConcurrentPro)
	}
	if ConcurrentScale != 500 {
		t.Errorf("ConcurrentScale = %d, want 500", ConcurrentScale)
	}
}

func TestPlanConstants_Retention(t *testing.T) {
	t.Parallel()
	if RetentionFree != 1 {
		t.Errorf("RetentionFree = %d, want 1", RetentionFree)
	}
	if RetentionStarter != 7 {
		t.Errorf("RetentionStarter = %d, want 7", RetentionStarter)
	}
	if RetentionPro != 30 {
		t.Errorf("RetentionPro = %d, want 30", RetentionPro)
	}
	if RetentionScale != 60 {
		t.Errorf("RetentionScale = %d, want 60", RetentionScale)
	}
	if RetentionEnterprise != 90 {
		t.Errorf("RetentionEnterprise = %d, want 90", RetentionEnterprise)
	}
}

func TestPlanConstants_OrgLimits(t *testing.T) {
	t.Parallel()
	if MaxOrgsFree != 1 {
		t.Errorf("MaxOrgsFree = %d, want 1", MaxOrgsFree)
	}
	if MaxOrgsStarter != 2 {
		t.Errorf("MaxOrgsStarter = %d, want 2", MaxOrgsStarter)
	}
	if MaxOrgsPro != 5 {
		t.Errorf("MaxOrgsPro = %d, want 5", MaxOrgsPro)
	}
	if MaxOrgsScale != 10 {
		t.Errorf("MaxOrgsScale = %d, want 10", MaxOrgsScale)
	}
}

func TestPlanConstants_ProjectLimits(t *testing.T) {
	t.Parallel()
	if MaxProjectsFree != 1 {
		t.Errorf("MaxProjectsFree = %d, want 1", MaxProjectsFree)
	}
	if MaxProjectsStarter != 3 {
		t.Errorf("MaxProjectsStarter = %d, want 3", MaxProjectsStarter)
	}
	if MaxProjectsPro != 10 {
		t.Errorf("MaxProjectsPro = %d, want 10", MaxProjectsPro)
	}
	if MaxProjectsScale != 50 {
		t.Errorf("MaxProjectsScale = %d, want 50", MaxProjectsScale)
	}
}

func TestPlanConstants_MemberLimits(t *testing.T) {
	t.Parallel()
	if MaxMembersFree != 1 {
		t.Errorf("MaxMembersFree = %d, want 1", MaxMembersFree)
	}
	if MaxMembersStarter != 5 {
		t.Errorf("MaxMembersStarter = %d, want 5", MaxMembersStarter)
	}
	if MaxMembersPro != 10 {
		t.Errorf("MaxMembersPro = %d, want 10", MaxMembersPro)
	}
	if MaxMembersScale != 50 {
		t.Errorf("MaxMembersScale = %d, want 50", MaxMembersScale)
	}
}

func TestPlanConstants_SpendingLimits(t *testing.T) {
	t.Parallel()
	if MaxSpendingStarter != 500_000_000 {
		t.Errorf("MaxSpendingStarter = %d, want 500000000", MaxSpendingStarter)
	}
	if MaxSpendingPro != 2_000_000_000 {
		t.Errorf("MaxSpendingPro = %d, want 2000000000", MaxSpendingPro)
	}
	if MaxSpendingScale != 5_000_000_000 {
		t.Errorf("MaxSpendingScale = %d, want 5000000000", MaxSpendingScale)
	}
}

func TestPlanConstants_Overage(t *testing.T) {
	t.Parallel()
	if DefaultOveragePerKRunsMicrousd != 200_000 {
		t.Errorf("DefaultOveragePerKRunsMicrousd = %d, want 200000", DefaultOveragePerKRunsMicrousd)
	}
}

func TestPlanLimits_EnterpriseFeatures(t *testing.T) {
	t.Parallel()
	ent := GetPlanLimits(domain.PlanEnterprise)
	if !ent.HasDedicatedCompute {
		t.Error("Enterprise should have HasDedicatedCompute")
	}
	if !ent.HasStaticIPs {
		t.Error("Enterprise should have HasStaticIPs")
	}
	if !ent.HasVPCPeering {
		t.Error("Enterprise should have HasVPCPeering")
	}
	if !ent.HasSCIM {
		t.Error("Enterprise should have HasSCIM")
	}
	if !ent.HasDataResidency {
		t.Error("Enterprise should have HasDataResidency")
	}
	if !ent.HasCustomRBAC {
		t.Error("Enterprise should have HasCustomRBAC")
	}
	if !ent.HasReservedCapacity {
		t.Error("Enterprise should have HasReservedCapacity")
	}
	if !ent.HasPriorityQueue {
		t.Error("Enterprise should have HasPriorityQueue")
	}
	if !ent.HasIPAllowlisting {
		t.Error("Enterprise should have HasIPAllowlisting")
	}
	if !ent.HasSessionManagement {
		t.Error("Enterprise should have HasSessionManagement")
	}
	if !ent.HasSecretRotation {
		t.Error("Enterprise should have HasSecretRotation")
	}
	if !ent.HasSIEMExport {
		t.Error("Enterprise should have HasSIEMExport")
	}
	if !ent.HasSSO {
		t.Error("Enterprise should have HasSSO")
	}
	if !ent.HasSLA {
		t.Error("Enterprise should have HasSLA")
	}
}

func TestPlanLimits_NonEnterpriseNoEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter} {
		l := GetPlanLimits(tier)
		if l.HasDedicatedCompute {
			t.Errorf("%s should NOT have HasDedicatedCompute", tier)
		}
		if l.HasStaticIPs {
			t.Errorf("%s should NOT have HasStaticIPs", tier)
		}
		if l.HasSIEMExport {
			t.Errorf("%s should NOT have HasSIEMExport", tier)
		}
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
