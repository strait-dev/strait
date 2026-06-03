package billing

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

type launchPromiseStatus string

const (
	launchPromiseRuntime launchPromiseStatus = "runtime"
	launchPromiseMetered launchPromiseStatus = "metered"
	launchPromiseDisplay launchPromiseStatus = "display"
	launchPromiseRoadmap launchPromiseStatus = "roadmap"
)

type launchPromiseEvidence struct {
	promise     string
	status      launchPromiseStatus
	gate        string
	test        string
	roadmapGate Feature
}

var launchEnforcementMatrix = []launchPromiseEvidence{
	{promise: "monthly run allowance", status: launchPromiseRuntime, gate: "Enforcer.CheckMonthlyRunLimit", test: "TestCheckMonthlyRunLimit_PaidOverageDisabledHardCaps"},
	{promise: "Free overage requires payment method", status: launchPromiseRuntime, gate: "UsageService.SetOverageEnabled", test: "TestUsageService_SetOverageEnabled_FreeRequiresPaymentMethod"},
	{promise: "paid overage can be disabled", status: launchPromiseRuntime, gate: "Enforcer.CheckMonthlyRunLimit", test: "TestCheckMonthlyRunLimit_PaidOverageDisabledHardCaps"},
	{promise: "spending cap blocks and pauses schedules", status: launchPromiseRuntime, gate: "Enforcer.CheckSpendingLimit", test: "TestCheckSpendingLimit_DispatchesCapReachedAndOverageDisabled"},
	{promise: "spending cap raise resumes quota-paused jobs", status: launchPromiseRuntime, gate: "UsageService.SetSpendingLimit", test: "TestUsageService_SetSpendingLimit_RaisedAboveCurrentSpendResumesQuotaPausedJobs"},
	{promise: "concurrent run cap", status: launchPromiseRuntime, gate: "Enforcer.CheckConcurrentRunLimit", test: "TestEnforcer_CheckConcurrentRunLimit"},
	{promise: "worker connection cap", status: launchPromiseRuntime, gate: "Enforcer.ReserveWorkerConnection", test: "TestReserveWorkerConnection_EnforcesCapAcrossEnforcers"},
	{promise: "workflow step cap", status: launchPromiseRuntime, gate: "registry workflow registration gate", test: "TestWorkflowStepCap_"},
	{promise: "cron schedule count cap", status: launchPromiseRuntime, gate: "scheduler admission gate", test: "TestCronEnforcement"},
	{promise: "cron minimum interval cap", status: launchPromiseRuntime, gate: "scheduler cron validator", test: "TestCronMinimumInterval"},
	{promise: "project cap", status: launchPromiseRuntime, gate: "Enforcer.CheckProjectLimit", test: "TestEnforcer_CheckProjectLimit"},
	{promise: "member cap", status: launchPromiseRuntime, gate: "Enforcer.CheckMemberLimit", test: "TestEnforcer_CheckMemberLimit"},
	{promise: "webhook endpoint cap", status: launchPromiseRuntime, gate: "webhook endpoint admission", test: "TestWebhookEndpointLimit"},
	{promise: "environment cap", status: launchPromiseRuntime, gate: "environment admission", test: "TestEnvironmentLimit"},
	{promise: "history retention cap", status: launchPromiseRuntime, gate: "PlanRetentionResolver", test: "TestGetOrgRetentionDays_ProPlan"},
	{promise: "API rate limit", status: launchPromiseRuntime, gate: "ratelimit middleware", test: "TestAPIRateLimit"},
	{promise: "RBAC level", status: launchPromiseRuntime, gate: "RBACLevel plan limit", test: "TestFeatureGating_RBAC"},
	{promise: "audit logs Scale+", status: launchPromiseRuntime, gate: "FeatureAuditLogs", test: "TestLaunchCatalogKeepsRoadmapSecurityFeaturesInactive"},
	{promise: "canary deployments Scale+", status: launchPromiseRuntime, gate: "FeatureCanaryDeployments", test: "TestFeatureGating_CanaryDeployments"},
	{promise: "approval gates Pro+", status: launchPromiseRuntime, gate: "FeatureApprovalGates", test: "TestFeatureGating_ApprovalGates"},
	{promise: "sub-workflows Pro+", status: launchPromiseRuntime, gate: "FeatureSubWorkflows", test: "TestFeatureGating_SubWorkflows"},
	{promise: "job chaining Pro+", status: launchPromiseRuntime, gate: "FeatureJobChaining", test: "TestFeatureGating_JobChaining"},
	{promise: "compensating transactions Pro+", status: launchPromiseRuntime, gate: "FeatureCompensatingTxns", test: "TestFeatureGating_CompensatingTxns"},
	{promise: "SLA target flag", status: launchPromiseDisplay, gate: "FeatureSLA", test: "TestPlanLimits_EnterpriseUnlimitedFeatures"},
	{promise: "overage metering to Stripe", status: launchPromiseMetered, gate: "worker recordTerminalRunBilling", test: "TestBillingEnforcement_TerminalFailureRecordsBillableRunCost"},
	{promise: "SSO roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSSO},
	{promise: "SCIM roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSCIM},
	{promise: "IP allowlisting roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureIPAllowlisting},
	{promise: "static IPs roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureStaticIPs},
	{promise: "VPC peering roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureVPCPeering},
	{promise: "data residency roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureDataResidency},
	{promise: "custom RBAC roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureCustomRBAC},
	{promise: "dedicated compute roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureDedicatedCompute},
	{promise: "priority queue roadmap", status: launchPromiseRoadmap, roadmapGate: FeaturePriorityQueue},
}

func TestLaunchEnforcementMatrixHasEvidenceForActivePromises(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, row := range launchEnforcementMatrix {
		if row.promise == "" {
			t.Fatal("launch enforcement matrix contains an unnamed promise")
		}
		if seen[row.promise] {
			t.Fatalf("duplicate launch enforcement promise %q", row.promise)
		}
		seen[row.promise] = true

		switch row.status {
		case launchPromiseRuntime, launchPromiseMetered, launchPromiseDisplay:
			if strings.TrimSpace(row.gate) == "" {
				t.Fatalf("%q is %s but has no gate evidence", row.promise, row.status)
			}
			if !strings.HasPrefix(row.test, "Test") {
				t.Fatalf("%q is %s but has no test evidence: %q", row.promise, row.status, row.test)
			}
		case launchPromiseRoadmap:
			if row.roadmapGate == "" {
				t.Fatalf("%q is roadmap but has no roadmap feature gate", row.promise)
			}
		default:
			t.Fatalf("%q has unknown launch status %q", row.promise, row.status)
		}
	}
}

func TestLaunchEnforcementMatrixRoadmapFeaturesStayInactive(t *testing.T) {
	t.Parallel()

	registry := NewStaticRegistry()
	for _, row := range launchEnforcementMatrix {
		if row.status != launchPromiseRoadmap {
			continue
		}
		for _, tier := range domain.AllPlanTiers() {
			if registry.AllowsFeature(tier, row.roadmapGate) {
				t.Fatalf("%s enables roadmap feature %q for %s", row.promise, row.roadmapGate, tier)
			}
		}
	}
}
