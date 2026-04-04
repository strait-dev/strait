package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestIsPresetAllowed_FreeTier(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	allowed := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x"}
	blocked := []string{"large-1x", "large-2x"}

	for _, p := range allowed {
		if !free.IsPresetAllowed(p) {
			t.Errorf("Free.IsPresetAllowed(%q) = false, want true", p)
		}
	}
	for _, p := range blocked {
		if free.IsPresetAllowed(p) {
			t.Errorf("Free.IsPresetAllowed(%q) = true, want false", p)
		}
	}
}

func TestIsPresetAllowed_PaidTiers(t *testing.T) {
	t.Parallel()

	allPresets := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}

	for _, tier := range []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanEnterprise} {
		limits := GetPlanLimits(tier)
		for _, p := range allPresets {
			if !limits.IsPresetAllowed(p) {
				t.Errorf("%s.IsPresetAllowed(%q) = false, want true", tier, p)
			}
		}
	}
}

func TestIsPresetAllowed_UnknownPreset(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	if free.IsPresetAllowed("nonexistent") {
		t.Error("Free.IsPresetAllowed(nonexistent) = true, want false")
	}

	pro := GetPlanLimits(domain.PlanPro)
	if !pro.IsPresetAllowed("nonexistent") {
		t.Error("Pro.IsPresetAllowed(nonexistent) = true (nil AllowedPresets = all)")
	}
}

func TestIsPresetAllowed_EmptyString(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	if free.IsPresetAllowed("") {
		t.Error("Free.IsPresetAllowed(\"\") = true, want false (empty not in allowed list)")
	}
}

func TestIsPresetAllowed_CaseSensitive(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	if free.IsPresetAllowed("MICRO") {
		t.Error("Free.IsPresetAllowed(MICRO) = true, want false (case-sensitive)")
	}
	if free.IsPresetAllowed("Micro") {
		t.Error("Free.IsPresetAllowed(Micro) = true, want false (case-sensitive)")
	}
}

func TestFeatureGating_ApprovalGates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier domain.PlanTier
		want bool
	}{
		{domain.PlanFree, false},
		{domain.PlanStarter, false},
		{domain.PlanPro, true},
		{domain.PlanScale, true},
		{domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		limits := GetPlanLimits(tt.tier)
		if limits.HasApprovalGates != tt.want {
			t.Errorf("%s.HasApprovalGates = %v, want %v", tt.tier, limits.HasApprovalGates, tt.want)
		}
	}
}

func TestFeatureGating_SubWorkflows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier domain.PlanTier
		want bool
	}{
		{domain.PlanFree, false},
		{domain.PlanStarter, false},
		{domain.PlanPro, true},
		{domain.PlanScale, true},
		{domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		limits := GetPlanLimits(tt.tier)
		if limits.HasSubWorkflows != tt.want {
			t.Errorf("%s.HasSubWorkflows = %v, want %v", tt.tier, limits.HasSubWorkflows, tt.want)
		}
	}
}

func TestFeatureGating_CanaryDeployments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier domain.PlanTier
		want bool
	}{
		{domain.PlanFree, false},
		{domain.PlanStarter, false},
		{domain.PlanPro, false},
		{domain.PlanScale, true},
		{domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		limits := GetPlanLimits(tt.tier)
		if limits.HasCanaryDeployments != tt.want {
			t.Errorf("%s.HasCanaryDeployments = %v, want %v", tt.tier, limits.HasCanaryDeployments, tt.want)
		}
	}
}

func TestFeatureGating_CompensatingTxns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier domain.PlanTier
		want bool
	}{
		{domain.PlanFree, false},
		{domain.PlanStarter, false},
		{domain.PlanPro, true},
		{domain.PlanScale, true},
		{domain.PlanEnterprise, true},
	}

	for _, tt := range tests {
		limits := GetPlanLimits(tt.tier)
		if limits.HasCompensatingTxns != tt.want {
			t.Errorf("%s.HasCompensatingTxns = %v, want %v", tt.tier, limits.HasCompensatingTxns, tt.want)
		}
	}
}

func TestFeatureGating_JobChaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier      domain.PlanTier
		wantChain bool
		wantDepth int
	}{
		{domain.PlanFree, false, 0},
		{domain.PlanStarter, false, 0},
		{domain.PlanPro, true, 10},
		{domain.PlanScale, true, 10},
		{domain.PlanEnterprise, true, -1},
	}

	for _, tt := range tests {
		limits := GetPlanLimits(tt.tier)
		if limits.HasJobChaining != tt.wantChain {
			t.Errorf("%s.HasJobChaining = %v, want %v", tt.tier, limits.HasJobChaining, tt.wantChain)
		}
		if limits.MaxJobChainDepth != tt.wantDepth {
			t.Errorf("%s.MaxJobChainDepth = %d, want %d", tt.tier, limits.MaxJobChainDepth, tt.wantDepth)
		}
	}
}

func TestDAGStepLimit_ExactBoundary(t *testing.T) {
	t.Parallel()

	for _, tier := range domain.AllPlanTiers() {
		limits := GetPlanLimits(tier)
		if limits.MaxWorkflowDAGSteps == -1 {
			continue // unlimited, no boundary
		}
		if limits.MaxWorkflowDAGSteps <= 0 {
			t.Errorf("%s.MaxWorkflowDAGSteps = %d, want > 0", tier, limits.MaxWorkflowDAGSteps)
		}
	}
}

func TestCronOverlapPolicy_FreeTier(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	if free.AllCronOverlapPolicies {
		t.Error("Free.AllCronOverlapPolicies = true, want false (allow only)")
	}

	for _, tier := range []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanEnterprise} {
		limits := GetPlanLimits(tier)
		if !limits.AllCronOverlapPolicies {
			t.Errorf("%s.AllCronOverlapPolicies = false, want true", tier)
		}
	}
}

func FuzzIsPresetAllowed(f *testing.F) {
	f.Add("micro")
	f.Add("large-2x")
	f.Add("")
	f.Add("MICRO")
	f.Add("nonexistent\x00")

	f.Fuzz(func(t *testing.T, preset string) {
		for _, tier := range domain.AllPlanTiers() {
			limits := GetPlanLimits(tier)
			// Should never panic.
			_ = limits.IsPresetAllowed(preset)
		}
	})
}

func FuzzFeatureGating(f *testing.F) {
	f.Add("free", "http_mode")
	f.Add("scale", "canary_deployments")
	f.Add("unknown", "nonexistent")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, tierStr, featureStr string) {
		reg := NewStaticRegistry()
		// Should never panic.
		_ = reg.AllowsFeature(domain.PlanTier(tierStr), Feature(featureStr))
	})
}
