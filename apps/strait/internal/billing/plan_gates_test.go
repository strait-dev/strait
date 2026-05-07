package billing

import (
	"testing"

	"strait/internal/domain"
)

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
