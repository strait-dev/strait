package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tt.
				wantDisplay, limits.DisplayName,
			)
			assert.Equal(t, tt.
				wantMonthly, limits.PriceMonthlyUsd,
			)
			assert.Equal(t, tt.
				wantConcurrent, limits.MaxConcurrentRuns,
			)
			assert.Equal(t, tt.
				wantProjects, limits.MaxProjectsPerOrg,
			)
			assert.Equal(t, tt.
				wantMembers, limits.MaxMembersPerOrg,
			)
			assert.Equal(t, tt.
				wantCreditCard, limits.RequiresCreditCard,
			)
			assert.Equal(t, tt.
				wantRetention, limits.RetentionDays,
			)
		})
	}
}

func TestGetPlanLimits_UnknownTier(t *testing.T) {
	t.Parallel()
	limits := GetPlanLimits(domain.PlanTier("unknown"))
	assert.Equal(t, domain.
		PlanFree, limits.PlanTier,
	)
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
			assert.Equal(t, tt.
				want, got)
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
			assert.True(t, limits.
				AllowsHTTPMode)
		})
	}
}

func TestPlanLimits_DailyRunsUnlimited(t *testing.T) {
	t.Parallel()
	for _, tier := range domain.AllPlanTiers() {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tier)
			assert.EqualValues(t, -1,
				limits.MaxRunsPerDay)
		})
	}
}

func TestHTTPCostPerRunMicrousd(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 20,

		HTTPCostPerRunMicrousd)
}

func TestAllPlansHaveEntries(t *testing.T) {
	t.Parallel()
	for _, tier := range domain.AllPlanTiers() {
		if _, ok := Plans[tier]; !ok {
			assert.Failf(t, "test failure",

				"missing plan definition for tier %q", tier)
		}
	}
	assert.Len(t, Plans,

		6)
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
			assert.Equal(t, tt.
				wantDAGSteps, l.MaxWorkflowDAGSteps,
			)
			assert.Equal(t, tt.
				wantApprovalGates, l.HasApprovalGates,
			)
			assert.Equal(t, tt.
				wantSubWorkflows, l.HasSubWorkflows,
			)
			assert.Equal(t, tt.
				wantJobChaining, l.HasJobChaining,
			)
			assert.Equal(t, tt.
				wantChainDepth, l.MaxJobChainDepth,
			)
			assert.Equal(t, tt.
				wantCompensation, l.HasCompensatingTxns,
			)
			assert.Equal(t, tt.
				wantCanary, l.HasCanaryDeployments,
			)
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
			assert.Equal(t, tt.
				wantScheduled, l.MaxScheduledJobs,
			)
			assert.Equal(t, tt.
				wantOverlapAll, l.AllCronOverlapPolicies,
			)
			assert.Equal(t, tt.
				wantEnvironments, l.MaxEnvironments,
			)
			assert.Equal(t, tt.
				wantWebhookEP, l.MaxWebhookEndpoints,
			)
			assert.Equal(t, tt.
				wantWebhookLevel, l.WebhookEventLevel,
			)
			assert.Equal(t, tt.
				wantAPIRate, l.APIRateLimit,
			)
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
			assert.Equal(t, tt.
				want, limits.HasAuditLogs,
			)
		})
	}
}

func TestPlanLimits_ScaleConstants(t *testing.T) {
	t.Parallel()

	scale := GetPlanLimits(domain.PlanScale)
	assert.Equal(t, PriceScaleMonthlyCents,

		scale.
			PriceMonthlyUsd,
	)
	assert.Equal(t, PriceScaleAnnualCents,

		scale.
			PriceAnnualUsd,
	)
	assert.Equal(t, ConcurrentScale,

		scale.MaxConcurrentRuns,
	)
	assert.Equal(t, ScaleOveragePerKMicrousd,

		scale.
			OveragePerKMicrousd)
}

func TestPlanConstants_Pricing(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1_900,

		PriceStarterMonthlyCents,
	)
	assert.Equal(t, 18_000,

		PriceStarterAnnualCents,
	)
	assert.Equal(t, 9_900,

		PriceProMonthlyCents)
	assert.Equal(t, 94_800,

		PriceProAnnualCents)
	assert.Equal(t, 29_900,

		PriceScaleMonthlyCents,
	)
	assert.Equal(t, 286_800,

		PriceScaleAnnualCents,
	)
	assert.Equal(t, 49_900,

		PriceBusinessMonthlyCents,
	)
	assert.Equal(t, 478_800,

		PriceBusinessAnnualCents,
	)
}

func TestPlanConstants_Credits(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 1_000_000,

		CreditFreeMicrousd,
	)
	assert.EqualValues(t, 19_000_000,

		CreditStarterMicrousd,
	)
	assert.EqualValues(t, 99_000_000,

		CreditProMicrousd,
	)
	assert.EqualValues(t, 299_000_000,

		CreditScaleMicrousd,
	)
	assert.EqualValues(t, 499_000_000,

		CreditBusinessMicrousd,
	)
}

func TestPlanConstants_Concurrent(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3,

		ConcurrentFree)
	assert.Equal(t, 15,

		ConcurrentStarter)
	assert.Equal(t, 100,

		ConcurrentPro)
	assert.Equal(t, 300,

		ConcurrentScale)
	assert.Equal(t, 500,

		ConcurrentBusiness)
}

func TestPlanConstants_Retention(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 7,

		RetentionFree)
	assert.Equal(t, 14,

		RetentionStarter)
	assert.Equal(t, 30,

		RetentionPro)
	assert.Equal(t, 60,

		RetentionScale)
	assert.Equal(t, 90,

		RetentionBusiness)
	assert.Equal(t, -1,
		RetentionEnterprise)
}

func TestPlanConstants_OrgLimits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1,

		MaxOrgsFree)
	assert.Equal(t, 2,

		MaxOrgsStarter)
	assert.Equal(t, 5,

		MaxOrgsPro)
	assert.Equal(t, 10,

		MaxOrgsScale)
}

func TestPlanConstants_ProjectLimits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1,

		MaxProjectsFree)
	assert.Equal(t, 3,

		MaxProjectsStarter)
	assert.Equal(t, 10,

		MaxProjectsPro)
	assert.Equal(t, 50,

		MaxProjectsScale)
}

func TestPlanConstants_MemberLimits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1,

		MaxMembersFree)
	assert.Equal(t, 3,

		MaxMembersStarter)
	assert.Equal(t, 10,

		MaxMembersPro)
	assert.Equal(t, 50,

		MaxMembersScale)
}

func TestPlanConstants_SpendingLimits(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 50_000_000,

		MaxSpendingFree)
	assert.EqualValues(t, 100_000_000,

		MaxSpendingStarter,
	)
	assert.EqualValues(t, 200_000_000,

		MaxSpendingPro)
	assert.EqualValues(t, 500_000_000,

		MaxSpendingScale,
	)
	assert.EqualValues(t, 1_500_000_000,

		MaxSpendingBusiness,
	)
}

func TestPlanConstants_Overage(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 500_000,

		DefaultOveragePerKMicrousd,
	)
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
		assert.False(t, tt.
			val)
	}
	assert.True(t, ent.
		HasSLA)
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
		require.False(t,
			limits.
				HasPriorityQueue)
		require.Equal(t, 10,
			limits.MaxDispatchPriority,
		)
	}
}

func TestPlanLimits_NonEnterpriseNoEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter} {
		l := GetPlanLimits(tier)
		assert.False(t, l.
			HasDedicatedCompute)
		assert.False(t, l.
			HasStaticIPs)
		assert.False(t, l.
			HasSIEMExport)
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
		assert.NotEmpty(t,

			limits.PlanTier)
	})
}
