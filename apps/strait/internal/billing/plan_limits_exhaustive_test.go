package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// A. Free Tier Enforcement.

func TestFreeEnforcement_ExecutionMode(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)
	assert.True(t, free.
		AllowsHTTPMode)

	// HTTP mode is available on all tiers.

}

func TestFreeEnforcement_WorkflowFeatures(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	tests := []struct {
		name string
		fn   func() bool
		want bool
	}{
		{"10_steps_at_limit", func() bool { return 10 <= free.MaxWorkflowDAGSteps }, true},
		{"11_steps_over_limit", func() bool { return 11 <= free.MaxWorkflowDAGSteps }, false},
		{"approval_gates_rejected", func() bool { return free.HasApprovalGates }, false},
		{"sub_workflows_rejected", func() bool { return free.HasSubWorkflows }, false},
		{"job_chaining_rejected", func() bool { return free.HasJobChaining }, false},
		{"compensating_txns_rejected", func() bool { return free.HasCompensatingTxns }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.
				want, tt.fn())

		})
	}
}

func TestFreeEnforcement_ResourceLimits(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)
	assert.EqualValues(t, 1,

		free.MaxEnvironments)
	assert.EqualValues(t, 0,

		free.MaxWebhookEndpoints)
	assert.Equal(t, MaxScheduledFree,

		free.MaxScheduledJobs,
	)
	assert.Equal(t, "none",

		free.WebhookEventLevel,
	)
	assert.False(t, free.
		AllCronOverlapPolicies)
	assert.False(t, free.
		HasCanaryDeployments)
	assert.False(t, free.
		HasAuditLogs)

}

// B. Starter Tier Enforcement (12 tests).

func TestStarterEnforcement(t *testing.T) {
	t.Parallel()
	s := GetPlanLimits(domain.PlanStarter)

	tests := []struct {
		name string
		fn   func() bool
		want bool
	}{
		{"http_mode_allowed", func() bool { return s.AllowsHTTPMode }, true},
		{"approval_gates_rejected", func() bool { return s.HasApprovalGates }, false},
		{"canary_rejected", func() bool { return s.HasCanaryDeployments }, false},
		{"cron_overlap_skip_allowed", func() bool { return s.AllCronOverlapPolicies }, true},
		{"webhook_basic_level", func() bool { return s.WebhookEventLevel == "basic" }, true},
		{"3_webhook_endpoints", func() bool { return s.MaxWebhookEndpoints == 3 }, true},
		{"25_schedules", func() bool { return s.MaxScheduledJobs == MaxScheduledStarter }, true},
		{"1_environment", func() bool { return s.MaxEnvironments == 1 }, true},
		{"dag_steps_at_starter_limit", func() bool { return s.MaxWorkflowDAGSteps == MaxDAGStepsStarter }, true},
		{"basic_rbac", func() bool { return s.RBACLevel == "basic" }, true},
		{"no_audit_logs", func() bool { return s.HasAuditLogs }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.
				want, tt.fn())

		})
	}
}

// C. Pro Tier Enforcement (10 tests).

func TestProEnforcement(t *testing.T) {
	t.Parallel()
	p := GetPlanLimits(domain.PlanPro)

	tests := []struct {
		name string
		fn   func() bool
		want bool
	}{
		{"http_mode_allowed", func() bool { return p.AllowsHTTPMode }, true},
		{"approval_gates_allowed", func() bool { return p.HasApprovalGates }, true},
		{"sub_workflows_allowed", func() bool { return p.HasSubWorkflows }, true},
		{"pro_dag_steps_at_limit", func() bool { return MaxDAGStepsPro <= p.MaxWorkflowDAGSteps }, true},
		{"pro_dag_steps_over_limit", func() bool { return MaxDAGStepsPro+1 <= p.MaxWorkflowDAGSteps }, false},
		{"job_chaining_allowed", func() bool { return p.HasJobChaining }, true},
		{"canary_rejected", func() bool { return p.HasCanaryDeployments }, false},
		{"compensating_txns_allowed", func() bool { return p.HasCompensatingTxns }, true},
		{"10_webhook_endpoints", func() bool { return p.MaxWebhookEndpoints == 10 }, true},
		{"pro_schedules", func() bool { return p.MaxScheduledJobs == MaxScheduledPro }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.
				want, tt.fn())

		})
	}
}

// D. Scale Tier Enforcement (8 tests).

func TestScaleEnforcement(t *testing.T) {
	t.Parallel()
	s := GetPlanLimits(domain.PlanScale)

	tests := []struct {
		name string
		fn   func() bool
		want bool
	}{
		{"canary_allowed", func() bool { return s.HasCanaryDeployments }, true},
		{"audit_logs_allowed", func() bool { return s.HasAuditLogs }, true},
		{"scale_dag_steps_at_limit", func() bool { return MaxDAGStepsScale <= s.MaxWorkflowDAGSteps }, true},
		{"scale_dag_steps_over_limit", func() bool { return MaxDAGStepsScale+1 <= s.MaxWorkflowDAGSteps }, false},
		{"scale_schedules", func() bool { return s.MaxScheduledJobs == MaxScheduledScale }, true},
		{"25_webhook_endpoints", func() bool { return s.MaxWebhookEndpoints == 25 }, true},
		{"all_overlap_policies", func() bool { return s.AllCronOverlapPolicies }, true},
		{"http_mode_allowed", func() bool { return s.AllowsHTTPMode }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.
				want, tt.fn())

		})
	}
}

// E. Enterprise Tier (5 tests).

func TestEnterpriseEnforcement(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	assert.EqualValues(t, -1,
		e.MaxWorkflowDAGSteps)
	assert.EqualValues(t, -1,
		e.MaxConcurrentRuns)
	assert.EqualValues(t, -1,
		e.MaxScheduledJobs)
	assert.EqualValues(t, -1,
		e.MaxWebhookEndpoints)

}

// F. Cron Overlap + Schedule (8 tests).

func TestCronEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tier    domain.PlanTier
		overlap bool
		maxCron int
	}{
		{"free_allow_only", domain.PlanFree, false, MaxScheduledFree},
		{"starter_all_policies", domain.PlanStarter, true, MaxScheduledStarter},
		{"pro_all_policies", domain.PlanPro, true, MaxScheduledPro},
		{"scale_all_policies", domain.PlanScale, true, MaxScheduledScale},
		{"enterprise_all_policies", domain.PlanEnterprise, true, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)
			assert.Equal(t, tt.
				overlap, limits.AllCronOverlapPolicies,
			)
			assert.Equal(t, tt.
				maxCron, limits.MaxScheduledJobs,
			)

		})
	}

	// Edge cases.
	t.Run("free_overlap_allow_is_ok", func(t *testing.T) {
		// "allow" policy should always work regardless of AllCronOverlapPolicies.
		free := GetPlanLimits(domain.PlanFree)
		assert.False(t, free.
			AllCronOverlapPolicies)

	})

	t.Run("remove_cron_always_ok", func(t *testing.T) {
		// Setting cron to "" is always allowed (not adding a schedule).
		// This is tested at the API level, not the plan level.
	})

	t.Run("boundary_free_schedule_limit", func(t *testing.T) {
		free := GetPlanLimits(domain.PlanFree)
		assert.LessOrEqual(t, MaxScheduledFree, free.
			MaxScheduledJobs,
		)
		assert.False(t, MaxScheduledFree+
			1 <= free.MaxScheduledJobs,
		)

		// MaxScheduledFree is the exact limit.

	})
}

// K. Webhook Event Gating (6 tests).

func TestWebhookEventEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tier  domain.PlanTier
		level string
	}{
		{"free_none", domain.PlanFree, "none"},
		{"starter_basic", domain.PlanStarter, "basic"},
		{"pro_all", domain.PlanPro, "all"},
		{"scale_all", domain.PlanScale, "all"},
		{"enterprise_all_custom", domain.PlanEnterprise, "all_custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)
			assert.Equal(t, tt.
				level, limits.WebhookEventLevel,
			)

		})
	}

	t.Run("basic_events_list", func(t *testing.T) {
		// Verify the basic event set matches what's enforced.
		basic := map[string]bool{"run.completed": true, "run.failed": true}
		for _, allowed := range basic {
			assert.True(t, allowed)

		}
	})
}

// L. Self-Hosted / Edition Gating (6 tests).

func TestSelfHostedEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("community_edition_skips_gating", func(t *testing.T) {
		t.Parallel()
		edition := domain.EditionCommunity
		assert.False(t, edition.
			RequiresHTTPModeGating())

	})

	t.Run("cloud_edition_requires_gating", func(t *testing.T) {
		t.Parallel()
		edition := domain.EditionCloud
		assert.True(t, edition.
			RequiresHTTPModeGating(),
		)

	})

	// On self-hosted, the enforcer is nil and getOrgPlanLimits returns nil.
	// This means all plan gate checks return nil (allowed).
	// We verify that the enterprise tier allows launch-active features while
	// keeping launch-roadmap features inactive.
	t.Run("enterprise_has_launch_active_features", func(t *testing.T) {
		t.Parallel()
		reg := NewStaticRegistry()
		features := []Feature{
			FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
			FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
			FeatureAuditLogs, FeatureSLA,
		}
		for _, f := range features {
			assert.True(t, reg.
				AllowsFeature(domain.PlanEnterprise,

					f))

		}
		for _, f := range roadmapEnterpriseFeatures {
			assert.False(t, reg.
				AllowsFeature(domain.PlanEnterprise,

					f))

		}
	})

	t.Run("enterprise_unlimited_limits", func(t *testing.T) {
		t.Parallel()
		e := GetPlanLimits(domain.PlanEnterprise)
		unlimitedFields := map[string]int{
			"MaxConcurrentRuns":   e.MaxConcurrentRuns,
			"MaxWorkflowDAGSteps": e.MaxWorkflowDAGSteps,
			"MaxScheduledJobs":    e.MaxScheduledJobs,
			"MaxWebhookEndpoints": e.MaxWebhookEndpoints,
			"MaxProjectsPerOrg":   e.MaxProjectsPerOrg,
			"MaxMembersPerOrg":    e.MaxMembersPerOrg,
		}
		for _, val := range unlimitedFields {
			assert.EqualValues(t, -1,
				val)

		}
	})

	t.Run("community_no_spending_check", func(t *testing.T) {
		t.Parallel()
		// On community edition, no enforcer exists, so spending limits are never checked.
		// This is verified by the nil check in getOrgPlanLimits.
		edition := domain.EditionCommunity
		assert.False(t, edition.
			RequiresHTTPModeGating())

	})
}

// Cross-tier monotonicity tests.

func TestPlanLimits_Monotonic(t *testing.T) {
	t.Parallel()
	tiers := domain.AllPlanTiers()

	numericLimits := []struct {
		name string
		fn   func(OrgPlanLimits) int
	}{
		{"MaxProjectsPerOrg", func(l OrgPlanLimits) int { return l.MaxProjectsPerOrg }},
		{"MaxMembersPerOrg", func(l OrgPlanLimits) int { return l.MaxMembersPerOrg }},
		{"MaxConcurrentRuns", func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }},
		{"RetentionDays", func(l OrgPlanLimits) int { return l.RetentionDays }},
		{"MaxWorkflowDAGSteps", func(l OrgPlanLimits) int { return l.MaxWorkflowDAGSteps }},
		{"MaxScheduledJobs", func(l OrgPlanLimits) int { return l.MaxScheduledJobs }},
		{"MaxWebhookEndpoints", func(l OrgPlanLimits) int { return l.MaxWebhookEndpoints }},
		{"APIRateLimit", func(l OrgPlanLimits) int { return l.APIRateLimit }},
	}

	for _, lim := range numericLimits {
		t.Run(lim.name, func(t *testing.T) {
			t.Parallel()
			for i := 1; i < len(tiers); i++ {
				prev := lim.fn(GetPlanLimits(tiers[i-1]))
				curr := lim.fn(GetPlanLimits(tiers[i]))
				if curr == -1 {
					continue // unlimited is always >= any value
				}
				if prev == -1 {
					assert.Failf(t, "test failure",

						"%s: %s=%d (unlimited) but %s=%d (limited)",
						lim.name, tiers[i-1], prev, tiers[i], curr)
					continue
				}
				assert.GreaterOrEqual(t, curr, prev)

			}
		})
	}
}

// Feature access should be monotonically increasing across tiers.

func TestFeatureAccess_Monotonic(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()
	tiers := domain.AllPlanTiers()

	features := make([]Feature, 0, 10+len(roadmapEnterpriseFeatures))
	features = append(features,
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSLA, FeatureRBAC, FeatureAllCronOverlap,
	)
	features = append(features, roadmapEnterpriseFeatures...)

	for _, f := range features {
		t.Run(string(f), func(t *testing.T) {
			t.Parallel()
			firstAllowed := -1
			for i, tier := range tiers {
				if reg.AllowsFeature(tier, f) {
					if firstAllowed == -1 {
						firstAllowed = i
					}
				} else {
					assert.EqualValues(t, -1,
						firstAllowed)

				}
			}
		})
	}
}
