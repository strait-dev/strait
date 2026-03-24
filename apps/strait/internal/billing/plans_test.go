package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestGetPlanLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tier           domain.PlanTier
		wantDisplay    string
		wantMonthly    int
		wantRunsPerDay int64
		wantConcurrent int
		wantProjects   int
		wantMembers    int
		wantCreditCard bool
		wantRetention  int
	}{
		{
			name:           "free",
			tier:           domain.PlanFree,
			wantDisplay:    "Free",
			wantMonthly:    0,
			wantRunsPerDay: 5000,
			wantConcurrent: 5,
			wantProjects:   2,
			wantMembers:    3,
			wantCreditCard: false,
			wantRetention:  1,
		},
		{
			name:           "starter",
			tier:           domain.PlanStarter,
			wantDisplay:    "Starter",
			wantMonthly:    1999,
			wantRunsPerDay: 25000,
			wantConcurrent: 25,
			wantProjects:   5,
			wantMembers:    10,
			wantCreditCard: true,
			wantRetention:  7,
		},
		{
			name:           "pro",
			tier:           domain.PlanPro,
			wantDisplay:    "Pro",
			wantMonthly:    4999,
			wantRunsPerDay: 100000,
			wantConcurrent: 100,
			wantProjects:   15,
			wantMembers:    25,
			wantCreditCard: true,
			wantRetention:  30,
		},
		{
			name:           "enterprise",
			tier:           domain.PlanEnterprise,
			wantDisplay:    "Enterprise",
			wantMonthly:    0,
			wantRunsPerDay: -1,
			wantConcurrent: -1,
			wantProjects:   -1,
			wantMembers:    -1,
			wantCreditCard: false,
			wantRetention:  90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := GetPlanLimits(tt.tier)

			if limits.DisplayName != tt.wantDisplay {
				t.Errorf("DisplayName = %q, want %q", limits.DisplayName, tt.wantDisplay)
			}
			if limits.PriceMonthlyUsd != tt.wantMonthly {
				t.Errorf("PriceMonthlyUsd = %d, want %d", limits.PriceMonthlyUsd, tt.wantMonthly)
			}
			if limits.MaxRunsPerDay != tt.wantRunsPerDay {
				t.Errorf("MaxRunsPerDay = %d, want %d", limits.MaxRunsPerDay, tt.wantRunsPerDay)
			}
			if limits.MaxConcurrentRuns != tt.wantConcurrent {
				t.Errorf("MaxConcurrentRuns = %d, want %d", limits.MaxConcurrentRuns, tt.wantConcurrent)
			}
			if limits.MaxProjectsPerOrg != tt.wantProjects {
				t.Errorf("MaxProjectsPerOrg = %d, want %d", limits.MaxProjectsPerOrg, tt.wantProjects)
			}
			if limits.MaxMembersPerOrg != tt.wantMembers {
				t.Errorf("MaxMembersPerOrg = %d, want %d", limits.MaxMembersPerOrg, tt.wantMembers)
			}
			if limits.RequiresCreditCard != tt.wantCreditCard {
				t.Errorf("RequiresCreditCard = %v, want %v", limits.RequiresCreditCard, tt.wantCreditCard)
			}
			if limits.RetentionDays != tt.wantRetention {
				t.Errorf("RetentionDays = %d, want %d", limits.RetentionDays, tt.wantRetention)
			}
		})
	}
}

func TestGetPlanLimits_UnknownTier(t *testing.T) {
	t.Parallel()
	limits := GetPlanLimits(domain.PlanTier("unknown"))
	if limits.PlanTier != domain.PlanFree {
		t.Errorf("expected fallback to free, got %q", limits.PlanTier)
	}
}

func TestFreeTierLimits(t *testing.T) {
	t.Parallel()
	free := GetPlanLimits(domain.PlanFree)

	if free.FreeManagedRunsPerMonth != 100 {
		t.Errorf("FreeManagedRunsPerMonth = %d, want 100", free.FreeManagedRunsPerMonth)
	}
	if free.FreeManagedPreset != "micro" {
		t.Errorf("FreeManagedPreset = %q, want micro", free.FreeManagedPreset)
	}
	if free.FreeManagedMaxTimeout != 10 {
		t.Errorf("FreeManagedMaxTimeout = %d, want 10", free.FreeManagedMaxTimeout)
	}
	if free.ComputeCreditMicrousd != 0 {
		t.Errorf("ComputeCreditMicrousd = %d, want 0", free.ComputeCreditMicrousd)
	}
}

func TestPaidTierCredits(t *testing.T) {
	t.Parallel()

	starter := GetPlanLimits(domain.PlanStarter)
	if starter.ComputeCreditMicrousd != 19990000 {
		t.Errorf("Starter credit = %d, want 19990000", starter.ComputeCreditMicrousd)
	}

	pro := GetPlanLimits(domain.PlanPro)
	if pro.ComputeCreditMicrousd != 49990000 {
		t.Errorf("Pro credit = %d, want 49990000", pro.ComputeCreditMicrousd)
	}
}

func TestIsDowngrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from domain.PlanTier
		to   domain.PlanTier
		want bool
	}{
		{"pro_to_starter", domain.PlanPro, domain.PlanStarter, true},
		{"pro_to_free", domain.PlanPro, domain.PlanFree, true},
		{"starter_to_free", domain.PlanStarter, domain.PlanFree, true},
		{"enterprise_to_pro", domain.PlanEnterprise, domain.PlanPro, true},
		{"enterprise_to_free", domain.PlanEnterprise, domain.PlanFree, true},
		{"starter_to_pro", domain.PlanStarter, domain.PlanPro, false},
		{"free_to_starter", domain.PlanFree, domain.PlanStarter, false},
		{"free_to_pro", domain.PlanFree, domain.PlanPro, false},
		{"free_to_enterprise", domain.PlanFree, domain.PlanEnterprise, false},
		{"same_free", domain.PlanFree, domain.PlanFree, false},
		{"same_pro", domain.PlanPro, domain.PlanPro, false},
		{"same_enterprise", domain.PlanEnterprise, domain.PlanEnterprise, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsDowngrade(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsDowngrade(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestAllPlansHaveEntries(t *testing.T) {
	t.Parallel()
	tiers := []domain.PlanTier{
		domain.PlanFree,
		domain.PlanStarter,
		domain.PlanPro,
		domain.PlanEnterprise,
	}
	for _, tier := range tiers {
		if _, ok := Plans[tier]; !ok {
			t.Errorf("missing plan definition for tier %q", tier)
		}
	}
}
