package billing

import (
	"reflect"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseLimits_AllUnlimited(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)

	unlimited := []struct {
		name string
		val  int
	}{
		{"MaxConcurrentRuns", e.MaxConcurrentRuns},
		{"MaxProjectsPerOrg", e.MaxProjectsPerOrg},
		{"MaxMembersPerOrg", e.MaxMembersPerOrg},
		{"MaxOrgsPerUser", e.MaxOrgsPerUser},
		{"MaxScheduledJobs", e.MaxScheduledJobs},
		{"MaxWebhookEndpoints", e.MaxWebhookEndpoints},
		{"MaxWorkflowDAGSteps", e.MaxWorkflowDAGSteps},
		{"APIRateLimit", e.APIRateLimit},
		{"MaxWebhookSubsPerProj", e.MaxWebhookSubsPerProj},
		{"MaxLogDrainsPerOrg", e.MaxLogDrainsPerOrg},
		{"MaxNotificationChannels", e.MaxNotificationChannels},
		{"MaxJobChainDepth", e.MaxJobChainDepth},
	}
	for _, tt := range unlimited {
		assert.EqualValues(t, -1, tt.val)

	}
	assert.EqualValues(t, -1, e.MaxRunsPerDay)

}

func TestEnterpriseLimits_RetentionUnlimited(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1, e.RetentionDays)

}

func TestEnterpriseLimits_RoadmapFeatureFlagsInactive(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)

	flags := []struct {
		name string
		val  bool
	}{
		{"HasDedicatedCompute", e.HasDedicatedCompute},
		{"HasStaticIPs", e.HasStaticIPs},
		{"HasVPCPeering", e.HasVPCPeering},
		{"HasSCIM", e.HasSCIM},
		{"HasDataResidency", e.HasDataResidency},
		{"HasCustomRBAC", e.HasCustomRBAC},
		{"HasPriorityQueue", e.HasPriorityQueue},
		{"HasIPAllowlisting", e.HasIPAllowlisting},
		{"HasSessionManagement", e.HasSessionManagement},
		{"HasSecretRotation", e.HasSecretRotation},
		{"HasSIEMExport", e.HasSIEMExport},
	}
	for _, tt := range flags {
		assert.False(t, tt.
			val)

	}
}

func TestEnterpriseLimits_ExistingFeatureFlags(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.False(t, e.
		HasSSO)
	assert.True(t, e.
		HasSLA)
	assert.True(t, e.
		HasAuditLogs,
	)
	assert.False(t, !e.HasRBAC ||

		e.RBACLevel != "full")
	assert.True(t, e.
		AllowsHTTPMode,
	)
	assert.True(t, e.
		HasApprovalGates,
	)
	assert.True(t, e.
		HasSubWorkflows,
	)
	assert.True(t, e.
		HasJobChaining,
	)
	assert.True(t, e.
		HasCompensatingTxns,
	)
	assert.True(t, e.
		HasCanaryDeployments,
	)

}

func TestEnterpriseLimits_NoPricing(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, 0,
		e.PriceMonthlyUsd,
	)
	assert.EqualValues(t, 0,
		e.PriceAnnualUsd,
	)

}

func TestEnterpriseLimits_NoRequiredCreditCard(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.False(t, e.
		RequiresCreditCard,
	)

}

func TestEnterpriseLimits_LaunchDefaultRegion(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.True(t, reflect.
		DeepEqual(e.AllowedRegions, []string{"iad"}))

}

func TestEnterpriseLimits_DedicatedSupportLevel(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.Equal(t, "dedicated",

		e.SupportLevel)

}

func TestEnterpriseLimits_NoSelfServeAddonPacks(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	require.Nil(t, e.MaxAddonPacks)

}

func TestNonEnterpriseTiers_NoEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
		limits := GetPlanLimits(tier)
		flags := []struct {
			name string
			val  bool
		}{
			{"HasDedicatedCompute", limits.HasDedicatedCompute},
			{"HasStaticIPs", limits.HasStaticIPs},
			{"HasVPCPeering", limits.HasVPCPeering},
			{"HasSCIM", limits.HasSCIM},
			{"HasDataResidency", limits.HasDataResidency},
			{"HasCustomRBAC", limits.HasCustomRBAC},
			{"HasPriorityQueue", limits.HasPriorityQueue},
			{"HasIPAllowlisting", limits.HasIPAllowlisting},
			{"HasSessionManagement", limits.HasSessionManagement},
			{"HasSecretRotation", limits.HasSecretRotation},
			{"HasSIEMExport", limits.HasSIEMExport},
		}
		for _, tt := range flags {
			assert.False(t, tt.
				val)

		}
	}
}

func TestNonEnterpriseTiers_NoSSO(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise} {
		assert.False(t, GetPlanLimits(tier).HasSSO)

	}
}

func TestNonEnterpriseTiers_NoSLA(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
		assert.False(t, GetPlanLimits(tier).HasSLA)

	}
}
