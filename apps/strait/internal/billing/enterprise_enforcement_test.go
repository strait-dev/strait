package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestEnforcement_Enterprise_NoConcurrentRunLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxConcurrentRuns != -1 {
		t.Errorf("MaxConcurrentRuns = %d, want -1 (unlimited)", e.MaxConcurrentRuns)
	}
}

func TestEnforcement_Enterprise_NoDailyRunLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxRunsPerDay != -1 {
		t.Errorf("MaxRunsPerDay = %d, want -1 (unlimited)", e.MaxRunsPerDay)
	}
}

func TestEnforcement_Enterprise_NoProjectLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxProjectsPerOrg != -1 {
		t.Errorf("MaxProjectsPerOrg = %d, want -1 (unlimited)", e.MaxProjectsPerOrg)
	}
}

func TestEnforcement_Enterprise_NoMemberLimit(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxMembersPerOrg != -1 {
		t.Errorf("MaxMembersPerOrg = %d, want -1 (unlimited)", e.MaxMembersPerOrg)
	}
}

func TestEnforcement_Enterprise_AllPresetsAllowed(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	presets := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}
	for _, p := range presets {
		if !e.IsPresetAllowed(p) {
			t.Errorf("Enterprise.IsPresetAllowed(%q) = false", p)
		}
	}
}

func TestEnforcement_Enterprise_HTTPModeAllowed(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if !e.AllowsHTTPMode {
		t.Error("Enterprise.AllowsHTTPMode = false, want true")
	}
}

func TestEnforcement_Enterprise_NoSpendingLimitCap(t *testing.T) {
	t.Parallel()
	// MaxSpendingLimit for enterprise should be -1 (uncapped).
	maxSpend := MaxSpendingLimit(domain.PlanEnterprise)
	if maxSpend != -1 {
		t.Errorf("MaxSpendingLimit(enterprise) = %d, want -1 (uncapped)", maxSpend)
	}
}

func TestEnforcement_Enterprise_CannotBypassViaInvalidTierString(t *testing.T) {
	t.Parallel()
	// Attempting to use "ENTERPRISE" (uppercase) should fall back to free tier.
	limits := GetPlanLimits(domain.PlanTier("ENTERPRISE"))
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("PlanTier for 'ENTERPRISE' = %q, want free (case-sensitive)", limits.PlanTier)
	}
	if limits.HasSSO {
		t.Error("'ENTERPRISE' should not have SSO (falls back to free)")
	}
}

func TestEnforcement_Enterprise_PlanDowngradeFromEnterprise(t *testing.T) {
	t.Parallel()
	// Downgrade from enterprise to any lower tier should be detected.
	for _, target := range []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanScale} {
		if !IsDowngrade(domain.PlanEnterprise, target) {
			t.Errorf("Enterprise -> %s should be a downgrade", target)
		}
	}
}

func TestEnforcement_Enterprise_CaseSensitiveTierLookup(t *testing.T) {
	t.Parallel()
	// Only lowercase "enterprise" is valid.
	valid := domain.PlanTier("enterprise")
	if !valid.IsValid() {
		t.Error("'enterprise' should be valid")
	}

	invalid := domain.PlanTier("Enterprise")
	if invalid.IsValid() {
		t.Error("'Enterprise' should not be valid (case sensitive)")
	}
}

func TestEnforcement_Enterprise_UnlimitedScheduledJobs(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxScheduledJobs != -1 {
		t.Errorf("MaxScheduledJobs = %d, want -1", e.MaxScheduledJobs)
	}
}

func TestEnforcement_Enterprise_UnlimitedWebhookEndpoints(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.MaxWebhookEndpoints != -1 {
		t.Errorf("MaxWebhookEndpoints = %d, want -1", e.MaxWebhookEndpoints)
	}
}

func TestEnforcement_Enterprise_CustomWebhookEventLevel(t *testing.T) {
	t.Parallel()
	e := GetPlanLimits(domain.PlanEnterprise)
	if e.WebhookEventLevel != "all_custom" {
		t.Errorf("WebhookEventLevel = %q, want all_custom", e.WebhookEventLevel)
	}
}
