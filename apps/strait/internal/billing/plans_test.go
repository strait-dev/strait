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
			wantConcurrent: ConcurrentFree,
			wantProjects:   MaxProjectsFree,
			wantMembers:    MaxMembersFree,
			wantCreditCard: false,
			wantRetention:  RetentionFree,
		},
		{
			name:           "starter",
			tier:           domain.PlanStarter,
			wantDisplay:    "Starter",
			wantMonthly:    PriceStarterMonthlyCents,
			wantConcurrent: ConcurrentStarter,
			wantProjects:   MaxProjectsStarter,
			wantMembers:    MaxMembersStarter,
			wantCreditCard: true,
			wantRetention:  RetentionStarter,
		},
		{
			name:           "pro",
			tier:           domain.PlanPro,
			wantDisplay:    "Pro",
			wantMonthly:    PriceProMonthlyCents,
			wantConcurrent: ConcurrentPro,
			wantProjects:   MaxProjectsPro,
			wantMembers:    MaxMembersPro,
			wantCreditCard: true,
			wantRetention:  RetentionPro,
		},
		{
			name:           "scale",
			tier:           domain.PlanScale,
			wantDisplay:    "Scale",
			wantMonthly:    PriceScaleMonthlyCents,
			wantConcurrent: ConcurrentScale,
			wantProjects:   MaxProjectsScale,
			wantMembers:    MaxMembersScale,
			wantCreditCard: true,
			wantRetention:  RetentionScale,
		},
		{
			name:           "business",
			tier:           domain.PlanBusiness,
			wantDisplay:    "Business",
			wantMonthly:    PriceBusinessMonthlyCents,
			wantConcurrent: ConcurrentBusiness,
			wantProjects:   -1,
			wantMembers:    -1,
			wantCreditCard: true,
			wantRetention:  RetentionBusiness,
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
			wantRetention:  -1,
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

	// All tiers have HTTP mode available; gating is done in plans.go.
	for _, tier := range domain.AllPlanTiers() {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tier)
			if !limits.AllowsHTTPMode {
				t.Errorf("%s.AllowsHTTPMode = false, want true", tier)
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
	if len(Plans) != 6 {
		t.Errorf("expected 6 plan entries, got %d", len(Plans))
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
		{domain.PlanFree, MaxDAGStepsFree, false, false, false, 0, false, false},
		{domain.PlanStarter, MaxDAGStepsStarter, false, false, false, 0, false, false},
		{domain.PlanPro, MaxDAGStepsPro, true, true, true, 10, true, false},
		{domain.PlanScale, MaxDAGStepsScale, true, true, true, 10, true, true},
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
		tier             domain.PlanTier
		wantScheduled    int
		wantOverlapAll   bool
		wantEnvironments int
		wantWebhookEP    int
		wantWebhookLevel string
		wantAPIRate      int
	}{
		{domain.PlanFree, MaxScheduledFree, false, 1, 0, "none", APIRateFree},
		{domain.PlanStarter, MaxScheduledStarter, true, 1, 3, "basic", APIRateStarter},
		{domain.PlanPro, MaxScheduledPro, true, 3, 10, "all", APIRatePro},
		{domain.PlanScale, MaxScheduledScale, true, 10, 25, "all", APIRateScale},
		{domain.PlanBusiness, -1, true, -1, -1, "all", -1},
		{domain.PlanEnterprise, -1, true, -1, -1, "all_custom", -1},
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
		})
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
	if scale.OveragePerKMicrousd != ScaleOveragePerKMicrousd {
		t.Errorf("OveragePerKMicrousd = %d, want %d", scale.OveragePerKMicrousd, ScaleOveragePerKMicrousd)
	}
}

func TestPlanConstants_Pricing(t *testing.T) {
	t.Parallel()
	if PriceStarterMonthlyCents != 1_900 {
		t.Errorf("PriceStarterMonthlyCents = %d, want 1900", PriceStarterMonthlyCents)
	}
	if PriceStarterAnnualCents != 18_000 {
		t.Errorf("PriceStarterAnnualCents = %d, want 18000", PriceStarterAnnualCents)
	}
	if PriceProMonthlyCents != 9_900 {
		t.Errorf("PriceProMonthlyCents = %d, want 9900", PriceProMonthlyCents)
	}
	if PriceProAnnualCents != 94_800 {
		t.Errorf("PriceProAnnualCents = %d, want 94800", PriceProAnnualCents)
	}
	if PriceScaleMonthlyCents != 29_900 {
		t.Errorf("PriceScaleMonthlyCents = %d, want 29900", PriceScaleMonthlyCents)
	}
	if PriceScaleAnnualCents != 286_800 {
		t.Errorf("PriceScaleAnnualCents = %d, want 286800", PriceScaleAnnualCents)
	}
	if PriceBusinessMonthlyCents != 49_900 {
		t.Errorf("PriceBusinessMonthlyCents = %d, want 49900", PriceBusinessMonthlyCents)
	}
	if PriceBusinessAnnualCents != 478_800 {
		t.Errorf("PriceBusinessAnnualCents = %d, want 478800", PriceBusinessAnnualCents)
	}
}

func TestPlanConstants_Credits(t *testing.T) {
	t.Parallel()
	if CreditFreeMicrousd != 1_000_000 {
		t.Errorf("CreditFreeMicrousd = %d, want 1000000", CreditFreeMicrousd)
	}
	if CreditStarterMicrousd != 19_000_000 {
		t.Errorf("CreditStarterMicrousd = %d, want 19000000", CreditStarterMicrousd)
	}
	if CreditProMicrousd != 99_000_000 {
		t.Errorf("CreditProMicrousd = %d, want 99000000", CreditProMicrousd)
	}
	if CreditScaleMicrousd != 299_000_000 {
		t.Errorf("CreditScaleMicrousd = %d, want 299000000", CreditScaleMicrousd)
	}
	if CreditBusinessMicrousd != 499_000_000 {
		t.Errorf("CreditBusinessMicrousd = %d, want 499000000", CreditBusinessMicrousd)
	}
}

func TestPlanConstants_Concurrent(t *testing.T) {
	t.Parallel()
	if ConcurrentFree != 3 {
		t.Errorf("ConcurrentFree = %d, want 3", ConcurrentFree)
	}
	if ConcurrentStarter != 15 {
		t.Errorf("ConcurrentStarter = %d, want 15", ConcurrentStarter)
	}
	if ConcurrentPro != 100 {
		t.Errorf("ConcurrentPro = %d, want 100", ConcurrentPro)
	}
	if ConcurrentScale != 300 {
		t.Errorf("ConcurrentScale = %d, want 300", ConcurrentScale)
	}
	if ConcurrentBusiness != 500 {
		t.Errorf("ConcurrentBusiness = %d, want 500", ConcurrentBusiness)
	}
}

func TestPlanConstants_Retention(t *testing.T) {
	t.Parallel()
	if RetentionFree != 7 {
		t.Errorf("RetentionFree = %d, want 7", RetentionFree)
	}
	if RetentionStarter != 14 {
		t.Errorf("RetentionStarter = %d, want 14", RetentionStarter)
	}
	if RetentionPro != 30 {
		t.Errorf("RetentionPro = %d, want 30", RetentionPro)
	}
	if RetentionScale != 60 {
		t.Errorf("RetentionScale = %d, want 60", RetentionScale)
	}
	if RetentionBusiness != 90 {
		t.Errorf("RetentionBusiness = %d, want 90", RetentionBusiness)
	}
	if RetentionEnterprise != -1 {
		t.Errorf("RetentionEnterprise = %d, want -1 (unlimited)", RetentionEnterprise)
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
	if MaxMembersStarter != 3 {
		t.Errorf("MaxMembersStarter = %d, want 3", MaxMembersStarter)
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
	if MaxSpendingFree != 50_000_000 {
		t.Errorf("MaxSpendingFree = %d, want 50000000", MaxSpendingFree)
	}
	if MaxSpendingStarter != 100_000_000 {
		t.Errorf("MaxSpendingStarter = %d, want 100000000", MaxSpendingStarter)
	}
	if MaxSpendingPro != 200_000_000 {
		t.Errorf("MaxSpendingPro = %d, want 200000000", MaxSpendingPro)
	}
	if MaxSpendingScale != 500_000_000 {
		t.Errorf("MaxSpendingScale = %d, want 500000000", MaxSpendingScale)
	}
	if MaxSpendingBusiness != 1_500_000_000 {
		t.Errorf("MaxSpendingBusiness = %d, want 1500000000", MaxSpendingBusiness)
	}
}

func TestPlanConstants_Overage(t *testing.T) {
	t.Parallel()
	if DefaultOveragePerKMicrousd != 500_000 {
		t.Errorf("DefaultOveragePerKMicrousd = %d, want 500000", DefaultOveragePerKMicrousd)
	}
}

func TestPlanLimits_EnterpriseRoadmapFeaturesInactive(t *testing.T) {
	t.Parallel()
	ent := GetPlanLimits(domain.PlanEnterprise)
	flags := []struct {
		name string
		val  bool
	}{
		{"HasDedicatedCompute", ent.HasDedicatedCompute},
		{"HasStaticIPs", ent.HasStaticIPs},
		{"HasVPCPeering", ent.HasVPCPeering},
		{"HasSCIM", ent.HasSCIM},
		{"HasDataResidency", ent.HasDataResidency},
		{"HasCustomRBAC", ent.HasCustomRBAC},
		{"HasPriorityQueue", ent.HasPriorityQueue},
		{"HasIPAllowlisting", ent.HasIPAllowlisting},
		{"HasSessionManagement", ent.HasSessionManagement},
		{"HasSecretRotation", ent.HasSecretRotation},
		{"HasSIEMExport", ent.HasSIEMExport},
		{"HasSSO", ent.HasSSO},
	}
	for _, tt := range flags {
		if tt.val {
			t.Errorf("Enterprise.%s = true, want false for launch roadmap item", tt.name)
		}
	}
	if !ent.HasSLA {
		t.Error("Enterprise should have HasSLA")
	}
}

func TestPlanLimits_DispatchPriorityIsNotPaidLaunchEntitlement(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{
		domain.PlanFree,
		domain.PlanStarter,
		domain.PlanPro,
		domain.PlanScale,
		domain.PlanBusiness,
		domain.PlanEnterprise,
	} {
		limits := GetPlanLimits(tier)
		if limits.HasPriorityQueue {
			t.Fatalf("%s HasPriorityQueue = true, want false for launch roadmap item", tier)
		}
		if limits.MaxDispatchPriority != 10 {
			t.Fatalf("%s MaxDispatchPriority = %d, want shared launch cap 10", tier, limits.MaxDispatchPriority)
		}
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
