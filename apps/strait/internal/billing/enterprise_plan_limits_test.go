package billing

import (
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
		{"MaxAlertRulesPerProj", e.MaxAlertRulesPerProj},
		{"MaxWebhookSubsPerProj", e.MaxWebhookSubsPerProj},
		{"MaxLogDrainsPerOrg", e.MaxLogDrainsPerOrg},
		{"MaxAIModelCallsPerDay", e.MaxAIModelCallsPerDay},
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

func TestEnterpriseLimits_Retention90Days(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.RetentionDays != 90 {
		t.Errorf("Enterprise.RetentionDays = %d, want 90", e.RetentionDays)
	}
}

func TestEnterpriseLimits_AllEnterpriseFeatureFlags(t *testing.T) {
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
		{"HasReservedCapacity", e.HasReservedCapacity},
		{"HasPriorityQueue", e.HasPriorityQueue},
		{"HasIPAllowlisting", e.HasIPAllowlisting},
		{"HasSessionManagement", e.HasSessionManagement},
		{"HasSecretRotation", e.HasSecretRotation},
		{"HasSIEMExport", e.HasSIEMExport},
	}
	for _, tt := range flags {
		if !tt.val {
			t.Errorf("Enterprise.%s = false, want true", tt.name)
		}
	}
}

func TestEnterpriseLimits_ExistingFeatureFlags(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)

	if !e.HasSSO {
		t.Error("Enterprise.HasSSO = false, want true")
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

func TestEnterpriseLimits_AllPresetsAllowed(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.AllowedPresets != nil {
		t.Error("Enterprise.AllowedPresets should be nil (all presets)")
	}
	for _, preset := range []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"} {
		if !e.IsPresetAllowed(preset) {
			t.Errorf("Enterprise.IsPresetAllowed(%q) = false", preset)
		}
	}
}

func TestEnterpriseLimits_AllRegions(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.AllowedRegions != nil {
		t.Error("Enterprise.AllowedRegions should be nil (all regions)")
	}
}

func TestEnterpriseLimits_DedicatedSupportLevel(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.SupportLevel != "dedicated" {
		t.Errorf("Enterprise.SupportLevel = %q, want dedicated", e.SupportLevel)
	}
}

func TestEnterpriseLimits_UnlimitedAddonPacks(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	for _, addonType := range AllAddonTypes() {
		max, ok := e.MaxAddonPacks[addonType]
		if !ok {
			t.Errorf("Enterprise.MaxAddonPacks missing %q", addonType)
			continue
		}
		if max != -1 {
			t.Errorf("Enterprise.MaxAddonPacks[%q] = %d, want -1", addonType, max)
		}
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
			{"HasReservedCapacity", limits.HasReservedCapacity},
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
	for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
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
