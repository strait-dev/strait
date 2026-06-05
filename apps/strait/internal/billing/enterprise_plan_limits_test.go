package billing

import (
	"reflect"
	"testing"

	"strait/internal/domain"
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
		if tt.val != -1 {
			t.Errorf("Enterprise.%s = %d, want -1 (unlimited)", tt.name, tt.val)
		}
	}

	if e.MaxRunsPerDay != -1 {
		t.Errorf("Enterprise.MaxRunsPerDay = %d, want -1", e.MaxRunsPerDay)
	}
}

func TestEnterpriseLimits_RetentionUnlimited(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.RetentionDays != -1 {
		t.Errorf("Enterprise.RetentionDays = %d, want -1 (unlimited)", e.RetentionDays)
	}
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
		if tt.val {
			t.Errorf("Enterprise.%s = true, want false for launch roadmap item", tt.name)
		}
	}
}

func TestEnterpriseLimits_ExistingFeatureFlags(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)

	if e.HasSSO {
		t.Error("Enterprise.HasSSO = true, want false for launch")
	}
	if !e.HasSLA {
		t.Error("Enterprise.HasSLA = false, want true")
	}
	if !e.HasAuditLogs {
		t.Error("Enterprise.HasAuditLogs = false, want true")
	}
	if !e.HasRBAC || e.RBACLevel != "full" {
		t.Errorf("Enterprise RBAC = (%v, %q), want (true, full)", e.HasRBAC, e.RBACLevel)
	}
	if !e.AllowsHTTPMode {
		t.Error("Enterprise.AllowsHTTPMode = false, want true")
	}
	if !e.HasApprovalGates {
		t.Error("Enterprise.HasApprovalGates = false, want true")
	}
	if !e.HasSubWorkflows {
		t.Error("Enterprise.HasSubWorkflows = false, want true")
	}
	if !e.HasJobChaining {
		t.Error("Enterprise.HasJobChaining = false, want true")
	}
	if !e.HasCompensatingTxns {
		t.Error("Enterprise.HasCompensatingTxns = false, want true")
	}
	if !e.HasCanaryDeployments {
		t.Error("Enterprise.HasCanaryDeployments = false, want true")
	}
}

func TestEnterpriseLimits_NoPricing(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.PriceMonthlyUsd != 0 {
		t.Errorf("Enterprise.PriceMonthlyUsd = %d, want 0 (custom)", e.PriceMonthlyUsd)
	}
	if e.PriceAnnualUsd != 0 {
		t.Errorf("Enterprise.PriceAnnualUsd = %d, want 0 (custom)", e.PriceAnnualUsd)
	}
}

func TestEnterpriseLimits_NoRequiredCreditCard(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.RequiresCreditCard {
		t.Error("Enterprise.RequiresCreditCard = true, want false")
	}
}

func TestEnterpriseLimits_LaunchDefaultRegion(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if !reflect.DeepEqual(e.AllowedRegions, []string{"iad"}) {
		t.Errorf("Enterprise.AllowedRegions = %#v, want launch default region", e.AllowedRegions)
	}
}

func TestEnterpriseLimits_DedicatedSupportLevel(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.SupportLevel != "dedicated" {
		t.Errorf("Enterprise.SupportLevel = %q, want dedicated", e.SupportLevel)
	}
}

func TestEnterpriseLimits_NoSelfServeAddonPacks(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxAddonPacks != nil {
		t.Fatalf("Enterprise.MaxAddonPacks = %#v, want nil for custom contract terms", e.MaxAddonPacks)
	}
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
			if tt.val {
				t.Errorf("%s.%s = true, want false", tier, tt.name)
			}
		}
	}
}

func TestNonEnterpriseTiers_NoSSO(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise} {
		if GetPlanLimits(tier).HasSSO {
			t.Errorf("%s.HasSSO = true, want false", tier)
		}
	}
}

func TestNonEnterpriseTiers_NoSLA(t *testing.T) {
	t.Parallel()
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
		if GetPlanLimits(tier).HasSLA {
			t.Errorf("%s.HasSLA = true, want false", tier)
		}
	}
}
