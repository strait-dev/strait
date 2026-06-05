package api

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleGetPlansLaunchCatalog(t *testing.T) {
	srv := &Server{}
	out, err := srv.handleGetPlans(context.Background(), &GetPlansInput{})
	require.NoError(t, err)
	require.Len(t,
		out.Body.Plans,
		6)

	byTier := make(map[string]PlanResponse, len(out.Body.Plans))
	for _, plan := range out.Body.Plans {
		byTier[plan.Tier] = plan
		assertPlanResponseMatchesGeneratedCatalog(t, plan)
	}

	business := byTier["business"]
	require.NotEmpty(t, business.RoadmapFeatures)

	if want := billing.GetPlanCatalog(domain.PlanBusiness).RoadmapFeatures; !slices.Equal(business.RoadmapFeatures, want) {
		require.Failf(t, "test failure",

			"business roadmap features = %v, want generated catalog %v", business.RoadmapFeatures, want)
	}

	free := byTier["free"]
	require.False(t, free.HasLogStreaming)

	starter := byTier["starter"]
	require.True(
		t, starter.HasLogStreaming,
	)
	require.False(t, free.OverageDefaultEnabled)
	require.Equal(t, billing.MaxSpendingFree,

		free.DefaultSpendingCapMicrousd,
	)
	require.True(
		t, starter.OverageDefaultEnabled,
	)
	require.Equal(t, billing.MaxSpendingStarter,

		starter.
			DefaultSpendingCapMicrousd,
	)

	pro := byTier["pro"]
	require.Equal(t, billing.GetPlanLimits(domain.
		PlanPro,
	).MaxNotificationChannels,
		pro.MaxNotificationChannels,
	)

	enterprise := byTier["enterprise"]
	require.Equal(t, -1, enterprise.
		MaxRunsPerMonth,
	)
	require.Equal(t, billing.MaxSpendingEnterprise,

		enterprise.
			DefaultSpendingCapMicrousd,
	)
	require.Equal(t, -1, enterprise.
		MaxNotificationChannels,
	)

	if want := billing.GetPlanCatalog(domain.PlanEnterprise).RoadmapFeatures; !slices.Equal(enterprise.RoadmapFeatures, want) {
		require.Failf(t, "test failure",

			"enterprise roadmap features = %v, want generated catalog %v", enterprise.RoadmapFeatures, want)
	}

	raw, err := json.Marshal(out.Body)
	require.NoError(t, err)

	var decoded struct {
		Plans []map[string]any `json:"plans"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, plan := range decoded.Plans {
		if _, ok := plan["allowed_regions"]; ok {
			require.Failf(t, "test failure",

				"plan %q exposes launch-inactive allowed_regions", plan["tier"])
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
				require.Failf(t, "test failure",

					"plan %q exposes inactive roadmap field %q in active entitlement response", plan["tier"], inactive)
			}
		}
	}
}

func assertPlanResponseMatchesGeneratedCatalog(t *testing.T, plan PlanResponse) {
	t.Helper()

	tier := domain.PlanTier(plan.Tier)
	limits := billing.GetPlanLimits(tier)
	catalog := billing.GetPlanCatalog(tier)
	require.Equal(t, string(limits.
		PlanTier),
		plan.Tier,
	)
	require.Equal(t, limits.DisplayName,
		plan.
			DisplayName,
	)
	require.Equal(t, limits.PriceMonthlyUsd,

		plan.PriceMonthlyUSD,
	)
	require.Equal(t, limits.PriceAnnualUsd,

		plan.PriceAnnualUSD,
	)
	require.Equal(t, limits.MaxOrgsPerUser,

		plan.MaxOrgsPerUser,
	)
	require.Equal(t, limits.MaxProjectsPerOrg,

		plan.MaxProjectsPerOrg,
	)
	require.Equal(t, limits.MaxMembersPerOrg,

		plan.MaxMembersPerOrg,
	)
	require.Equal(t, limits.MaxRunsPerMonth,

		plan.MaxRunsPerMonth,
	)
	require.Equal(t, limits.MaxConcurrentRuns,

		plan.MaxConcurrentRuns,
	)
	require.Equal(t, limits.RetentionDays,
		plan.
			RetentionDays,
	)
	require.Equal(t, limits.MaxWebhookSubsPerProj,

		plan.
			MaxWebhookSubsPerProject)
	require.Equal(t, limits.MaxLogDrainsPerOrg,

		plan.MaxLogDrainsPerOrg,
	)
	require.Equal(t, limits.MaxNotificationChannels,

		plan.
			MaxNotificationChannels)
	require.Equal(t, limits.HasRBAC,
		plan.HasRBAC,
	)
	require.Equal(t, limits.RBACLevel,
		plan.
			RBACLevel)
	require.Equal(t, limits.HasAuditLogs,
		plan.
			HasAuditLogs,
	)
	require.Equal(t, limits.HasSLA,
		plan.HasSLA,
	)
	require.Equal(t, limits.LogStreamingEnabled,

		plan.HasLogStreaming,
	)
	require.Equal(t, limits.HasCanaryDeployments,

		plan.
			HasCanaryDeployments,
	)
	require.Equal(t, limits.HasApprovalGates,

		plan.HasApprovalGates,
	)
	require.Equal(t, limits.HasSubWorkflows,

		plan.HasSubWorkflows,
	)
	require.Equal(t, limits.HasJobChaining,

		plan.HasJobChaining,
	)
	require.Equal(t, limits.HasCompensatingTxns,

		plan.HasCompensatingTxns,
	)
	require.Equal(t, limits.RequiresCreditCard,

		plan.RequiresCreditCard,
	)
	require.Equal(t, limits.OveragePerKMicrousd,

		plan.OveragePerKRunsMicrousd,
	)
	require.Equal(t, catalog.OverageDefaultEnabled,

		plan.
			OverageDefaultEnabled)
	require.Equal(t, catalog.DefaultSpendingCapMicrousd,

		plan.DefaultSpendingCapMicrousd,
	)
	require.Equal(t, limits.SupportLevel,
		plan.
			SupportLevel,
	)
	require.Equal(t, limits.MaxEnvironments,

		plan.MaxEnvironments,
	)
	require.Equal(t, limits.MaxScheduledJobs,

		plan.MaxScheduledJobs,
	)
	require.Equal(t, limits.CronMinIntervalSec,

		plan.CronMinIntervalSec,
	)
	require.Equal(t, limits.MaxWebhookEndpoints,

		plan.MaxWebhookEndpoints,
	)
	require.Equal(t, limits.MaxWorkflowDAGSteps,

		plan.MaxWorkflowDAGSteps,
	)
	require.Equal(t, limits.APIRateLimit,
		plan.
			APIRateLimit,
	)
	require.Equal(t, limits.WorkerConnections,

		plan.WorkerConnections,
	)
	require.True(
		t, slices.Equal(plan.
			RoadmapFeatures,

			catalog.
				RoadmapFeatures))
}
