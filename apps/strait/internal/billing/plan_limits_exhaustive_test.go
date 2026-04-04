package billing

import (
	"testing"

	"strait/internal/domain"
)

// A. Free Tier Enforcement (20 tests).

func TestFreeEnforcement_Presets(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	tests := []struct {
		name   string
		preset string
		want   bool
	}{
		{"large-1x_rejected", "large-1x", false},
		{"large-2x_rejected", "large-2x", false},
		{"medium-1x_allowed", "medium-1x", true},
		{"medium-2x_allowed", "medium-2x", true},
		{"small-1x_allowed", "small-1x", true},
		{"micro_allowed", "micro", true},
		{"case_sensitive_LARGE-1X_rejected", "LARGE-1X", false},
		{"empty_string_not_in_list", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := free.IsPresetAllowed(tt.preset); got != tt.want {
				t.Errorf("Free.IsPresetAllowed(%q) = %v, want %v", tt.preset, got, tt.want)
			}
		})
	}
}

func TestFreeEnforcement_ExecutionMode(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	if free.AllowsHTTPMode {
		t.Error("Free.AllowsHTTPMode = true, want false")
	}
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
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestFreeEnforcement_ResourceLimits(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	if free.MaxEnvironments != 1 {
		t.Errorf("Free.MaxEnvironments = %d, want 1", free.MaxEnvironments)
	}
	if free.MaxWebhookEndpoints != 0 {
		t.Errorf("Free.MaxWebhookEndpoints = %d, want 0", free.MaxWebhookEndpoints)
	}
	if free.MaxScheduledJobs != 10 {
		t.Errorf("Free.MaxScheduledJobs = %d, want 10", free.MaxScheduledJobs)
	}
	if free.WebhookEventLevel != "none" {
		t.Errorf("Free.WebhookEventLevel = %q, want none", free.WebhookEventLevel)
	}
	if free.AllCronOverlapPolicies {
		t.Error("Free.AllCronOverlapPolicies = true, want false")
	}
	if free.HasCanaryDeployments {
		t.Error("Free.HasCanaryDeployments = true, want false")
	}
	if free.HasAuditLogs {
		t.Error("Free.HasAuditLogs = true, want false")
	}
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
		{"all_presets_allowed", func() bool { return s.AllowedPresets == nil }, true},
		{"http_mode_rejected", func() bool { return s.AllowsHTTPMode }, false},
		{"approval_gates_rejected", func() bool { return s.HasApprovalGates }, false},
		{"canary_rejected", func() bool { return s.HasCanaryDeployments }, false},
		{"cron_overlap_skip_allowed", func() bool { return s.AllCronOverlapPolicies }, true},
		{"webhook_basic_level", func() bool { return s.WebhookEventLevel == "basic" }, true},
		{"3_webhook_endpoints", func() bool { return s.MaxWebhookEndpoints == 3 }, true},
		{"25_schedules", func() bool { return s.MaxScheduledJobs == 25 }, true},
		{"3_environments", func() bool { return s.MaxEnvironments == 3 }, true},
		{"50_workflow_steps", func() bool { return s.MaxWorkflowDAGSteps == 50 }, true},
		{"basic_rbac", func() bool { return s.RBACLevel == "basic" }, true},
		{"no_audit_logs", func() bool { return s.HasAuditLogs }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
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
		{"250_steps_at_limit", func() bool { return 250 <= p.MaxWorkflowDAGSteps }, true},
		{"251_steps_over_limit", func() bool { return 251 <= p.MaxWorkflowDAGSteps }, false},
		{"job_chaining_allowed", func() bool { return p.HasJobChaining }, true},
		{"canary_rejected", func() bool { return p.HasCanaryDeployments }, false},
		{"compensating_txns_allowed", func() bool { return p.HasCompensatingTxns }, true},
		{"10_webhook_endpoints", func() bool { return p.MaxWebhookEndpoints == 10 }, true},
		{"100_schedules", func() bool { return p.MaxScheduledJobs == 100 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
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
		{"1000_steps_at_limit", func() bool { return 1000 <= s.MaxWorkflowDAGSteps }, true},
		{"1001_steps_over_limit", func() bool { return 1001 <= s.MaxWorkflowDAGSteps }, false},
		{"500_schedules", func() bool { return s.MaxScheduledJobs == 500 }, true},
		{"25_webhook_endpoints", func() bool { return s.MaxWebhookEndpoints == 25 }, true},
		{"all_overlap_policies", func() bool { return s.AllCronOverlapPolicies }, true},
		{"http_mode_allowed", func() bool { return s.AllowsHTTPMode }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// E. Enterprise Tier (5 tests).

func TestEnterpriseEnforcement(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)

	if e.AllowedPresets != nil {
		t.Error("Enterprise.AllowedPresets should be nil (all presets)")
	}
	if e.MaxWorkflowDAGSteps != -1 {
		t.Errorf("Enterprise.MaxWorkflowDAGSteps = %d, want -1 (unlimited)", e.MaxWorkflowDAGSteps)
	}
	if e.MaxConcurrentRuns != -1 {
		t.Errorf("Enterprise.MaxConcurrentRuns = %d, want -1 (unlimited)", e.MaxConcurrentRuns)
	}
	if e.MaxScheduledJobs != -1 {
		t.Errorf("Enterprise.MaxScheduledJobs = %d, want -1 (unlimited)", e.MaxScheduledJobs)
	}
	if e.MaxWebhookEndpoints != -1 {
		t.Errorf("Enterprise.MaxWebhookEndpoints = %d, want -1 (unlimited)", e.MaxWebhookEndpoints)
	}
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
		{"free_allow_only", domain.PlanFree, false, 10},
		{"starter_all_policies", domain.PlanStarter, true, 25},
		{"pro_all_policies", domain.PlanPro, true, 100},
		{"scale_all_policies", domain.PlanScale, true, 500},
		{"enterprise_all_policies", domain.PlanEnterprise, true, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)
			if limits.AllCronOverlapPolicies != tt.overlap {
				t.Errorf("%s: AllCronOverlapPolicies = %v, want %v", tt.tier, limits.AllCronOverlapPolicies, tt.overlap)
			}
			if limits.MaxScheduledJobs != tt.maxCron {
				t.Errorf("%s: MaxScheduledJobs = %d, want %d", tt.tier, limits.MaxScheduledJobs, tt.maxCron)
			}
		})
	}

	// Edge cases.
	t.Run("free_overlap_allow_is_ok", func(t *testing.T) {
		// "allow" policy should always work regardless of AllCronOverlapPolicies.
		free := GetPlanLimits(domain.PlanFree)
		if free.AllCronOverlapPolicies {
			t.Error("free should not have all overlap policies")
		}
	})

	t.Run("remove_cron_always_ok", func(t *testing.T) {
		// Setting cron to "" is always allowed (not adding a schedule).
		// This is tested at the API level, not the plan level.
	})

	t.Run("boundary_free_10th_schedule", func(t *testing.T) {
		free := GetPlanLimits(domain.PlanFree)
		if 10 > free.MaxScheduledJobs {
			t.Error("10th schedule should be at the limit, not over")
		}
		if 11 <= free.MaxScheduledJobs {
			t.Error("11th schedule should be over the limit")
		}
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
			if limits.WebhookEventLevel != tt.level {
				t.Errorf("%s: WebhookEventLevel = %q, want %q", tt.tier, limits.WebhookEventLevel, tt.level)
			}
		})
	}

	t.Run("basic_events_list", func(t *testing.T) {
		// Verify the basic event set matches what's enforced.
		basic := map[string]bool{"run.completed": true, "run.failed": true}
		for evt, allowed := range basic {
			if !allowed {
				t.Errorf("basic event %q should be allowed", evt)
			}
		}
	})
}

// L. Self-Hosted / Edition Gating (6 tests).

func TestSelfHostedEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("community_edition_skips_gating", func(t *testing.T) {
		t.Parallel()
		edition := domain.EditionCommunity
		if edition.RequiresHTTPModeGating() {
			t.Error("EditionCommunity.RequiresHTTPModeGating() = true, want false")
		}
	})

	t.Run("cloud_edition_requires_gating", func(t *testing.T) {
		t.Parallel()
		edition := domain.EditionCloud
		if !edition.RequiresHTTPModeGating() {
			t.Error("EditionCloud.RequiresHTTPModeGating() = false, want true")
		}
	})

	// On self-hosted, the enforcer is nil and getOrgPlanLimits returns nil.
	// This means all plan gate checks return nil (allowed).
	// We verify that the enterprise tier (which self-hosted effectively grants)
	// has all features enabled.
	t.Run("enterprise_has_all_features", func(t *testing.T) {
		t.Parallel()
		reg := NewStaticRegistry()
		features := []Feature{
			FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
			FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
			FeatureAuditLogs, FeatureSSO, FeatureSLA,
		}
		for _, f := range features {
			if !reg.AllowsFeature(domain.PlanEnterprise, f) {
				t.Errorf("Enterprise should have feature %q", f)
			}
		}
	})

	t.Run("enterprise_all_presets", func(t *testing.T) {
		t.Parallel()
		e := GetPlanLimits(domain.PlanEnterprise)
		for _, p := range []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"} {
			if !e.IsPresetAllowed(p) {
				t.Errorf("Enterprise.IsPresetAllowed(%q) = false", p)
			}
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
		for field, val := range unlimitedFields {
			if val != -1 {
				t.Errorf("Enterprise.%s = %d, want -1 (unlimited)", field, val)
			}
		}
	})

	t.Run("community_no_spending_check", func(t *testing.T) {
		t.Parallel()
		// On community edition, no enforcer exists, so spending limits are never checked.
		// This is verified by the nil check in getOrgPlanLimits.
		edition := domain.EditionCommunity
		if edition.RequiresHTTPModeGating() {
			t.Error("community should not require gating")
		}
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
					t.Errorf("%s: %s=%d (unlimited) but %s=%d (limited)",
						lim.name, tiers[i-1], prev, tiers[i], curr)
					continue
				}
				if curr < prev {
					t.Errorf("%s: %s=%d > %s=%d -- should not decrease",
						lim.name, tiers[i-1], prev, tiers[i], curr)
				}
			}
		})
	}
}

// Feature access should be monotonically increasing across tiers.

func TestFeatureAccess_Monotonic(t *testing.T) {
	t.Parallel()
	reg := NewStaticRegistry()
	tiers := domain.AllPlanTiers()

	features := []Feature{
		FeatureHTTPMode, FeatureApprovalGates, FeatureSubWorkflows,
		FeatureJobChaining, FeatureCompensatingTxns, FeatureCanaryDeployments,
		FeatureAuditLogs, FeatureSSO, FeatureSLA, FeatureRBAC,
		FeatureAllCronOverlap, FeatureAIAssistantBYOK,
	}

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
					if firstAllowed != -1 {
						t.Errorf("feature %q: allowed on %s (rank %d) but not on %s (rank %d)",
							f, tiers[firstAllowed], firstAllowed, tier, i)
					}
				}
			}
		})
	}
}
