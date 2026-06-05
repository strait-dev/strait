package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticRegistry_AllTiers(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	for _, tier := range domain.AllPlanTiers() {
		limits := reg.Get(tier)
		assert.Equal(t, tier,

			limits.PlanTier)
	}
}

func TestStaticRegistry_All(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()
	all := reg.All()
	require.Len(t, all,

		6)

	expected := domain.AllPlanTiers()
	for i, limits := range all {
		assert.Equal(t, expected[i], limits.PlanTier)
	}
}

func TestStaticRegistry_UnknownTier_ReturnsFree(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()
	limits := reg.Get(domain.PlanTier("nonexistent"))
	assert.Equal(t, domain.
		PlanFree, limits.PlanTier,
	)
}

func TestStaticRegistry_AllowsFeature(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	tests := []struct {
		tier    domain.PlanTier
		feature Feature
		want    bool
	}{
		// HTTP mode: all tiers.
		{domain.PlanFree, FeatureHTTPMode, true},
		{domain.PlanStarter, FeatureHTTPMode, true},
		{domain.PlanPro, FeatureHTTPMode, true},
		{domain.PlanScale, FeatureHTTPMode, true},
		{domain.PlanEnterprise, FeatureHTTPMode, true},
		// Approval gates: Pro+.
		{domain.PlanFree, FeatureApprovalGates, false},
		{domain.PlanStarter, FeatureApprovalGates, false},
		{domain.PlanPro, FeatureApprovalGates, true},
		{domain.PlanScale, FeatureApprovalGates, true},
		{domain.PlanEnterprise, FeatureApprovalGates, true},
		// Canary deployments: Scale+.
		{domain.PlanFree, FeatureCanaryDeployments, false},
		{domain.PlanStarter, FeatureCanaryDeployments, false},
		{domain.PlanPro, FeatureCanaryDeployments, false},
		{domain.PlanScale, FeatureCanaryDeployments, true},
		{domain.PlanEnterprise, FeatureCanaryDeployments, true},
		// Audit logs: Scale+.
		{domain.PlanFree, FeatureAuditLogs, false},
		{domain.PlanStarter, FeatureAuditLogs, false},
		{domain.PlanPro, FeatureAuditLogs, false},
		{domain.PlanScale, FeatureAuditLogs, true},
		{domain.PlanEnterprise, FeatureAuditLogs, true},
		// SSO: roadmap/contact-sales only at launch.
		{domain.PlanFree, FeatureSSO, false},
		{domain.PlanStarter, FeatureSSO, false},
		{domain.PlanPro, FeatureSSO, false},
		{domain.PlanScale, FeatureSSO, false},
		{domain.PlanEnterprise, FeatureSSO, false},
		// SLA: Enterprise only.
		{domain.PlanPro, FeatureSLA, false},
		{domain.PlanScale, FeatureSLA, false},
		{domain.PlanEnterprise, FeatureSLA, true},
		// Sub-workflows: Pro+.
		{domain.PlanStarter, FeatureSubWorkflows, false},
		{domain.PlanPro, FeatureSubWorkflows, true},
		// Job chaining: Pro+.
		{domain.PlanStarter, FeatureJobChaining, false},
		{domain.PlanPro, FeatureJobChaining, true},
		// Compensating transactions: Pro+.
		{domain.PlanStarter, FeatureCompensatingTxns, false},
		{domain.PlanPro, FeatureCompensatingTxns, true},
		// RBAC: Starter+.
		{domain.PlanFree, FeatureRBAC, false},
		{domain.PlanStarter, FeatureRBAC, true},
		// Cron overlap policies: Starter+.
		{domain.PlanFree, FeatureAllCronOverlap, false},
		{domain.PlanStarter, FeatureAllCronOverlap, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier)+"_"+string(tt.feature), func(t *testing.T) {
			t.Parallel()
			got := reg.AllowsFeature(tt.tier, tt.feature)
			assert.Equal(t, tt.
				want, got)
		})
	}
}

func TestStaticRegistry_AllowsFeature_InvalidFeature(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	for _, tier := range domain.AllPlanTiers() {
		assert.False(t, reg.
			AllowsFeature(tier, Feature("nonexistent_feature")))
	}
}

func TestStaticRegistry_MaxForLimit(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	tests := []struct {
		tier  domain.PlanTier
		limit LimitKey
		want  int
	}{
		{domain.PlanFree, LimitMaxProjectsPerOrg, 1},
		{domain.PlanStarter, LimitMaxProjectsPerOrg, 3},
		{domain.PlanPro, LimitMaxProjectsPerOrg, 10},
		{domain.PlanScale, LimitMaxProjectsPerOrg, 50},
		{domain.PlanEnterprise, LimitMaxProjectsPerOrg, -1},

		{domain.PlanFree, LimitMaxConcurrentRuns, ConcurrentFree},
		{domain.PlanScale, LimitMaxConcurrentRuns, ConcurrentScale},
		{domain.PlanEnterprise, LimitMaxConcurrentRuns, ConcurrentEnterprise},

		{domain.PlanFree, LimitMaxWorkflowDAGSteps, MaxDAGStepsFree},
		{domain.PlanStarter, LimitMaxWorkflowDAGSteps, MaxDAGStepsStarter},
		{domain.PlanPro, LimitMaxWorkflowDAGSteps, MaxDAGStepsPro},
		{domain.PlanScale, LimitMaxWorkflowDAGSteps, MaxDAGStepsScale},
		{domain.PlanEnterprise, LimitMaxWorkflowDAGSteps, -1},

		{domain.PlanFree, LimitMaxScheduledJobs, MaxScheduledFree},
		{domain.PlanScale, LimitMaxScheduledJobs, MaxScheduledScale},

		{domain.PlanFree, LimitMaxEnvironments, 1},
		{domain.PlanStarter, LimitMaxEnvironments, 1},

		{domain.PlanFree, LimitMaxWebhookEndpoints, 0},
		{domain.PlanStarter, LimitMaxWebhookEndpoints, 3},
		{domain.PlanPro, LimitMaxWebhookEndpoints, 10},
		{domain.PlanScale, LimitMaxWebhookEndpoints, 25},
		{domain.PlanEnterprise, LimitMaxWebhookEndpoints, -1},

		{domain.PlanFree, LimitAPIRateLimit, 60},
		{domain.PlanScale, LimitAPIRateLimit, 3000},
		{domain.PlanEnterprise, LimitAPIRateLimit, -1},

		{domain.PlanFree, LimitWorkerConnections, WorkerConnectionsFree},
		{domain.PlanStarter, LimitWorkerConnections, WorkerConnectionsStarter},
		{domain.PlanPro, LimitWorkerConnections, WorkerConnectionsPro},
		{domain.PlanScale, LimitWorkerConnections, WorkerConnectionsScale},
		{domain.PlanBusiness, LimitWorkerConnections, WorkerConnectionsBusiness},
		{domain.PlanEnterprise, LimitWorkerConnections, WorkerConnectionsEnterprise},

		{domain.PlanFree, LimitMaxDispatchPriority, MaxDispatchPriorityFree},
		{domain.PlanStarter, LimitMaxDispatchPriority, MaxDispatchPriorityStarter},
		{domain.PlanPro, LimitMaxDispatchPriority, MaxDispatchPriorityPro},
		{domain.PlanScale, LimitMaxDispatchPriority, MaxDispatchPriorityScale},
		{domain.PlanBusiness, LimitMaxDispatchPriority, MaxDispatchPriorityBusiness},
		{domain.PlanEnterprise, LimitMaxDispatchPriority, MaxDispatchPriorityEnterprise},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier)+"_"+string(tt.limit), func(t *testing.T) {
			t.Parallel()
			got := reg.MaxForLimit(tt.tier, tt.limit)
			assert.Equal(t, tt.
				want, got)
		})
	}
}

func TestStaticRegistry_MaxForLimit_InvalidKey(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	for _, tier := range domain.AllPlanTiers() {
		got := reg.MaxForLimit(tier, LimitKey("nonexistent_limit"))
		assert.Equal(t, 0,

			got)
	}
}

func TestStaticRegistry_FeatureGating_Exhaustive(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// Verify that no feature is accidentally available on a tier lower than its minimum.
	// HTTP mode is now available on all tiers; the following features remain Pro-gated.
	proFeatures := []Feature{
		FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns,
	}
	for _, f := range proFeatures {
		assert.False(t, reg.
			AllowsFeature(domain.PlanFree,

				f))
		assert.False(t, reg.
			AllowsFeature(domain.PlanStarter,

				f))
	}

	scaleFeatures := []Feature{FeatureCanaryDeployments, FeatureAuditLogs}
	for _, f := range scaleFeatures {
		assert.False(t, reg.
			AllowsFeature(domain.PlanFree,

				f))
		assert.False(t, reg.
			AllowsFeature(domain.PlanStarter,

				f))
		assert.False(t, reg.
			AllowsFeature(domain.PlanPro,

				f))
	}

	for _, f := range roadmapEnterpriseFeatures {
		for _, tier := range domain.AllPlanTiers() {
			assert.False(t, reg.
				AllowsFeature(tier, f))
		}
	}

	enterpriseFeatures := []Feature{FeatureSLA}
	for _, f := range enterpriseFeatures {
		for _, tier := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
			assert.False(t, reg.
				AllowsFeature(tier, f))
		}
	}
}

func TestStaticRegistry_LimitsMonotonicallyIncrease(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	// For numeric limits, verify that each tier has >= the previous tier.
	limits := []LimitKey{
		LimitMaxProjectsPerOrg, LimitMaxMembersPerOrg, LimitMaxConcurrentRuns,
		LimitRetentionDays, LimitMaxWorkflowDAGSteps, LimitMaxScheduledJobs,
		LimitMaxWebhookEndpoints, LimitAPIRateLimit, LimitWorkerConnections,
		LimitMaxDispatchPriority,
	}

	tiers := domain.AllPlanTiers()
	for _, lk := range limits {
		for i := 1; i < len(tiers); i++ {
			prev := reg.MaxForLimit(tiers[i-1], lk)
			curr := reg.MaxForLimit(tiers[i], lk)
			// -1 means unlimited, which is always >= any value.
			if curr == -1 {
				continue
			}
			if prev == -1 {
				assert.Failf(t, "test failure",

					"Limit %q: %s=%d (unlimited) but %s=%d (limited) -- should not decrease",
					lk, tiers[i-1], prev, tiers[i], curr)
				continue
			}
			assert.GreaterOrEqual(t, curr, prev)
		}
	}
}

func TestStaticRegistry_RequiredPlanForFeature(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()

	tests := []struct {
		feature Feature
		want    domain.PlanTier
	}{
		// Starter features.
		{FeatureRBAC, domain.PlanStarter},
		{FeatureAllCronOverlap, domain.PlanStarter},
		// HTTP mode is now available on all tiers (free is the first tier).
		{FeatureHTTPMode, domain.PlanFree},
		// Pro features.
		{FeatureApprovalGates, domain.PlanPro},
		{FeatureSubWorkflows, domain.PlanPro},
		{FeatureJobChaining, domain.PlanPro},
		{FeatureCompensatingTxns, domain.PlanPro},
		// Scale features.
		{FeatureAuditLogs, domain.PlanScale},
		{FeatureCanaryDeployments, domain.PlanScale},
		// Business features.
		{FeatureSLA, domain.PlanBusiness},
		// Roadmap/contact-sales features have no launch upgrade tier.
		{FeatureSSO, ""},
		{FeatureSCIM, ""},
		{FeatureDedicatedCompute, ""},
		// Unknown feature defaults to enterprise.
		{Feature("nonexistent"), domain.PlanEnterprise},
	}

	for _, tt := range tests {
		t.Run(string(tt.feature), func(t *testing.T) {
			t.Parallel()
			got := reg.RequiredPlanForFeature(tt.feature)
			assert.Equal(t, tt.
				want, got)
		})
	}
}

// Verify the registry satisfies the interface at compile time.
var _ PlanRegistry = (*StaticRegistry)(nil)
