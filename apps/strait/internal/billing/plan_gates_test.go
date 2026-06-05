package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, tt.
			want, limits.HasApprovalGates,
		)

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
		assert.Equal(t, tt.
			want, limits.HasSubWorkflows,
		)

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
		assert.Equal(t, tt.
			want, limits.HasCanaryDeployments,
		)

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
		assert.Equal(t, tt.
			want, limits.HasCompensatingTxns,
		)

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
		assert.Equal(t, tt.
			wantChain, limits.HasJobChaining,
		)
		assert.Equal(t, tt.
			wantDepth, limits.MaxJobChainDepth,
		)

	}
}

func TestDAGStepLimit_ExactBoundary(t *testing.T) {
	t.Parallel()

	for _, tier := range domain.AllPlanTiers() {
		limits := GetPlanLimits(tier)
		if limits.MaxWorkflowDAGSteps == -1 {
			continue // unlimited, no boundary
		}
		assert.False(t, limits.
			MaxWorkflowDAGSteps <=

			0,
		)

	}
}

func TestCronOverlapPolicy_FreeTier(t *testing.T) {
	t.Parallel()

	free := GetPlanLimits(domain.PlanFree)
	assert.False(t, free.
		AllCronOverlapPolicies)

	for _, tier := range []domain.PlanTier{domain.PlanStarter, domain.PlanPro, domain.PlanScale, domain.PlanEnterprise} {
		limits := GetPlanLimits(tier)
		assert.True(t, limits.
			AllCronOverlapPolicies,
		)

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
