package api

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

func TestHandleGetPlansLaunchCatalog(t *testing.T) {
	srv := &Server{}
	out, err := srv.handleGetPlans(context.Background(), &GetPlansInput{})
	if err != nil {
		t.Fatalf("handleGetPlans returned error: %v", err)
	}
	if len(out.Body.Plans) != 6 {
		t.Fatalf("plans length = %d, want 6", len(out.Body.Plans))
	}

	byTier := make(map[string]PlanResponse, len(out.Body.Plans))
	for _, plan := range out.Body.Plans {
		byTier[plan.Tier] = plan
		assertPlanResponseMatchesGeneratedCatalog(t, plan)
	}

	business := byTier["business"]
	if len(business.RoadmapFeatures) == 0 {
		t.Fatal("business roadmap features should be present for display only")
	}
	if want := billing.GetPlanCatalog(domain.PlanBusiness).RoadmapFeatures; !slices.Equal(business.RoadmapFeatures, want) {
		t.Fatalf("business roadmap features = %v, want generated catalog %v", business.RoadmapFeatures, want)
	}

	free := byTier["free"]
	if free.HasLogStreaming {
		t.Fatal("free plan should not advertise log streaming")
	}
	starter := byTier["starter"]
	if !starter.HasLogStreaming {
		t.Fatal("starter plan should advertise log streaming")
	}
	if free.OverageDefaultEnabled {
		t.Fatal("free plan should not expose overage as enabled by default")
	}
	if free.DefaultSpendingCapMicrousd != billing.MaxSpendingFree {
		t.Fatalf("free default spending cap = %d, want %d", free.DefaultSpendingCapMicrousd, billing.MaxSpendingFree)
	}
	if !starter.OverageDefaultEnabled {
		t.Fatal("starter plan should expose overage as enabled by default")
	}
	if starter.DefaultSpendingCapMicrousd != billing.MaxSpendingStarter {
		t.Fatalf("starter default spending cap = %d, want %d", starter.DefaultSpendingCapMicrousd, billing.MaxSpendingStarter)
	}

	pro := byTier["pro"]
	if pro.MaxNotificationChannels != billing.GetPlanLimits(domain.PlanPro).MaxNotificationChannels {
		t.Fatalf("pro notification channel cap = %d, want generated plan limit", pro.MaxNotificationChannels)
	}

	enterprise := byTier["enterprise"]
	if enterprise.MaxRunsPerMonth != -1 {
		t.Fatalf("enterprise max runs = %d, want unlimited", enterprise.MaxRunsPerMonth)
	}
	if enterprise.DefaultSpendingCapMicrousd != billing.MaxSpendingEnterprise {
		t.Fatalf("enterprise default spending cap = %d, want %d", enterprise.DefaultSpendingCapMicrousd, billing.MaxSpendingEnterprise)
	}
	if enterprise.MaxNotificationChannels != -1 {
		t.Fatalf("enterprise notification channel cap = %d, want unlimited", enterprise.MaxNotificationChannels)
	}
	if want := billing.GetPlanCatalog(domain.PlanEnterprise).RoadmapFeatures; !slices.Equal(enterprise.RoadmapFeatures, want) {
		t.Fatalf("enterprise roadmap features = %v, want generated catalog %v", enterprise.RoadmapFeatures, want)
	}

	raw, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal plans: %v", err)
	}
	var decoded struct {
		Plans []map[string]any `json:"plans"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal plans: %v", err)
	}
	for _, plan := range decoded.Plans {
		if _, ok := plan["allowed_regions"]; ok {
			t.Fatalf("plan %q exposes launch-inactive allowed_regions", plan["tier"])
		}

		for _, inactive := range []string{
			"has_sso",
			"has_scim",
			"has_ip_allowlisting",
			"has_static_ips",
			"has_vpc_peering",
			"has_data_residency",
			"has_dedicated_compute",
			"has_reserved_capacity",
			"has_priority_queue",
			"has_session_management",
			"has_secret_rotation",
			"has_siem_export",
		} {
			if _, ok := plan[inactive]; ok {
				t.Fatalf("plan %q exposes inactive roadmap field %q in active entitlement response", plan["tier"], inactive)
			}
		}
	}
}

func assertPlanResponseMatchesGeneratedCatalog(t *testing.T, plan PlanResponse) {
	t.Helper()

	tier := domain.PlanTier(plan.Tier)
	limits := billing.GetPlanLimits(tier)
	catalog := billing.GetPlanCatalog(tier)

	if plan.Tier != string(limits.PlanTier) {
		t.Fatalf("%s tier = %q, want %q", plan.Tier, plan.Tier, limits.PlanTier)
	}
	if plan.DisplayName != limits.DisplayName {
		t.Fatalf("%s display name = %q, want %q", plan.Tier, plan.DisplayName, limits.DisplayName)
	}
	if plan.PriceMonthlyUSD != limits.PriceMonthlyUsd {
		t.Fatalf("%s monthly price = %d, want %d", plan.Tier, plan.PriceMonthlyUSD, limits.PriceMonthlyUsd)
	}
	if plan.PriceAnnualUSD != limits.PriceAnnualUsd {
		t.Fatalf("%s annual price = %d, want %d", plan.Tier, plan.PriceAnnualUSD, limits.PriceAnnualUsd)
	}
	if plan.MaxOrgsPerUser != limits.MaxOrgsPerUser {
		t.Fatalf("%s org cap = %d, want %d", plan.Tier, plan.MaxOrgsPerUser, limits.MaxOrgsPerUser)
	}
	if plan.MaxProjectsPerOrg != limits.MaxProjectsPerOrg {
		t.Fatalf("%s project cap = %d, want %d", plan.Tier, plan.MaxProjectsPerOrg, limits.MaxProjectsPerOrg)
	}
	if plan.MaxMembersPerOrg != limits.MaxMembersPerOrg {
		t.Fatalf("%s member cap = %d, want %d", plan.Tier, plan.MaxMembersPerOrg, limits.MaxMembersPerOrg)
	}
	if plan.MaxRunsPerMonth != limits.MaxRunsPerMonth {
		t.Fatalf("%s monthly run cap = %d, want %d", plan.Tier, plan.MaxRunsPerMonth, limits.MaxRunsPerMonth)
	}
	if plan.MaxConcurrentRuns != limits.MaxConcurrentRuns {
		t.Fatalf("%s concurrency cap = %d, want %d", plan.Tier, plan.MaxConcurrentRuns, limits.MaxConcurrentRuns)
	}
	if plan.RetentionDays != limits.RetentionDays {
		t.Fatalf("%s retention days = %d, want %d", plan.Tier, plan.RetentionDays, limits.RetentionDays)
	}
	if plan.MaxWebhookSubsPerProject != limits.MaxWebhookSubsPerProj {
		t.Fatalf("%s webhook subscription cap = %d, want %d", plan.Tier, plan.MaxWebhookSubsPerProject, limits.MaxWebhookSubsPerProj)
	}
	if plan.MaxLogDrainsPerOrg != limits.MaxLogDrainsPerOrg {
		t.Fatalf("%s log drain cap = %d, want %d", plan.Tier, plan.MaxLogDrainsPerOrg, limits.MaxLogDrainsPerOrg)
	}
	if plan.MaxNotificationChannels != limits.MaxNotificationChannels {
		t.Fatalf("%s notification channel cap = %d, want %d", plan.Tier, plan.MaxNotificationChannels, limits.MaxNotificationChannels)
	}
	if plan.HasRBAC != limits.HasRBAC {
		t.Fatalf("%s RBAC flag = %v, want %v", plan.Tier, plan.HasRBAC, limits.HasRBAC)
	}
	if plan.RBACLevel != limits.RBACLevel {
		t.Fatalf("%s RBAC level = %q, want %q", plan.Tier, plan.RBACLevel, limits.RBACLevel)
	}
	if plan.HasAuditLogs != limits.HasAuditLogs {
		t.Fatalf("%s audit logs flag = %v, want %v", plan.Tier, plan.HasAuditLogs, limits.HasAuditLogs)
	}
	if plan.HasSLA != limits.HasSLA {
		t.Fatalf("%s SLA flag = %v, want %v", plan.Tier, plan.HasSLA, limits.HasSLA)
	}
	if plan.HasLogStreaming != limits.LogStreamingEnabled {
		t.Fatalf("%s log streaming flag = %v, want %v", plan.Tier, plan.HasLogStreaming, limits.LogStreamingEnabled)
	}
	if plan.HasCanaryDeployments != limits.HasCanaryDeployments {
		t.Fatalf("%s canary flag = %v, want %v", plan.Tier, plan.HasCanaryDeployments, limits.HasCanaryDeployments)
	}
	if plan.HasApprovalGates != limits.HasApprovalGates {
		t.Fatalf("%s approval gate flag = %v, want %v", plan.Tier, plan.HasApprovalGates, limits.HasApprovalGates)
	}
	if plan.HasSubWorkflows != limits.HasSubWorkflows {
		t.Fatalf("%s sub-workflow flag = %v, want %v", plan.Tier, plan.HasSubWorkflows, limits.HasSubWorkflows)
	}
	if plan.HasJobChaining != limits.HasJobChaining {
		t.Fatalf("%s job chaining flag = %v, want %v", plan.Tier, plan.HasJobChaining, limits.HasJobChaining)
	}
	if plan.HasCompensatingTxns != limits.HasCompensatingTxns {
		t.Fatalf("%s compensating transaction flag = %v, want %v", plan.Tier, plan.HasCompensatingTxns, limits.HasCompensatingTxns)
	}
	if plan.RequiresCreditCard != limits.RequiresCreditCard {
		t.Fatalf("%s credit-card flag = %v, want %v", plan.Tier, plan.RequiresCreditCard, limits.RequiresCreditCard)
	}
	if plan.OveragePerKRunsMicrousd != limits.OveragePerKMicrousd {
		t.Fatalf("%s overage rate = %d, want %d", plan.Tier, plan.OveragePerKRunsMicrousd, limits.OveragePerKMicrousd)
	}
	if plan.OverageDefaultEnabled != catalog.OverageDefaultEnabled {
		t.Fatalf("%s overage default = %v, want %v", plan.Tier, plan.OverageDefaultEnabled, catalog.OverageDefaultEnabled)
	}
	if plan.DefaultSpendingCapMicrousd != catalog.DefaultSpendingCapMicrousd {
		t.Fatalf("%s spending cap = %d, want %d", plan.Tier, plan.DefaultSpendingCapMicrousd, catalog.DefaultSpendingCapMicrousd)
	}
	if plan.SupportLevel != limits.SupportLevel {
		t.Fatalf("%s support level = %q, want %q", plan.Tier, plan.SupportLevel, limits.SupportLevel)
	}
	if plan.MaxEnvironments != limits.MaxEnvironments {
		t.Fatalf("%s environment cap = %d, want %d", plan.Tier, plan.MaxEnvironments, limits.MaxEnvironments)
	}
	if plan.MaxScheduledJobs != limits.MaxScheduledJobs {
		t.Fatalf("%s schedule cap = %d, want %d", plan.Tier, plan.MaxScheduledJobs, limits.MaxScheduledJobs)
	}
	if plan.CronMinIntervalSec != limits.CronMinIntervalSec {
		t.Fatalf("%s cron minimum interval = %d, want %d", plan.Tier, plan.CronMinIntervalSec, limits.CronMinIntervalSec)
	}
	if plan.MaxWebhookEndpoints != limits.MaxWebhookEndpoints {
		t.Fatalf("%s webhook endpoint cap = %d, want %d", plan.Tier, plan.MaxWebhookEndpoints, limits.MaxWebhookEndpoints)
	}
	if plan.MaxWorkflowDAGSteps != limits.MaxWorkflowDAGSteps {
		t.Fatalf("%s workflow step cap = %d, want %d", plan.Tier, plan.MaxWorkflowDAGSteps, limits.MaxWorkflowDAGSteps)
	}
	if plan.APIRateLimit != limits.APIRateLimit {
		t.Fatalf("%s API rate limit = %d, want %d", plan.Tier, plan.APIRateLimit, limits.APIRateLimit)
	}
	if plan.WorkerConnections != limits.WorkerConnections {
		t.Fatalf("%s worker connection cap = %d, want %d", plan.Tier, plan.WorkerConnections, limits.WorkerConnections)
	}
	if !slices.Equal(plan.RoadmapFeatures, catalog.RoadmapFeatures) {
		t.Fatalf("%s roadmap features = %v, want %v", plan.Tier, plan.RoadmapFeatures, catalog.RoadmapFeatures)
	}
}
