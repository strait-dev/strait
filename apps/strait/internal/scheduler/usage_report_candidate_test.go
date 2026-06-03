package scheduler

import (
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

func TestUsageReportCandidate_EligiblePaidOptInEndedPeriod(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	periodEnd := todayStart.Add(-24 * time.Hour).Add(6 * time.Hour)
	sub := &billing.OrgSubscription{
		PlanTier:          string(domain.PlanStarter),
		MonthlyUsageEmail: true,
		CurrentPeriodEnd:  &periodEnd,
	}

	candidate, ok := newUsageReportCandidate("org-1", sub, todayStart)
	if !ok {
		t.Fatal("expected eligible usage report candidate")
	}
	if candidate.orgID != "org-1" {
		t.Fatalf("orgID = %q, want org-1", candidate.orgID)
	}
	if candidate.sub != sub {
		t.Fatal("candidate did not preserve subscription pointer")
	}
	wantPeriodEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	if !candidate.periodEnd.Equal(wantPeriodEnd) {
		t.Fatalf("periodEnd = %s, want %s", candidate.periodEnd, wantPeriodEnd)
	}
}

func TestUsageReportCandidate_IneligibleRules(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	endedYesterday := todayStart.Add(-24 * time.Hour)
	endsToday := todayStart.Add(time.Hour)

	tests := []struct {
		name string
		sub  *billing.OrgSubscription
	}{
		{
			name: "nil subscription",
		},
		{
			name: "free plan",
			sub: &billing.OrgSubscription{
				PlanTier:          string(domain.PlanFree),
				MonthlyUsageEmail: true,
				CurrentPeriodEnd:  &endedYesterday,
			},
		},
		{
			name: "enterprise custom billing",
			sub: &billing.OrgSubscription{
				PlanTier:          string(domain.PlanEnterprise),
				MonthlyUsageEmail: true,
				CurrentPeriodEnd:  &endedYesterday,
			},
		},
		{
			name: "not opted in",
			sub: &billing.OrgSubscription{
				PlanTier:         string(domain.PlanStarter),
				CurrentPeriodEnd: &endedYesterday,
			},
		},
		{
			name: "missing period end",
			sub: &billing.OrgSubscription{
				PlanTier:          string(domain.PlanStarter),
				MonthlyUsageEmail: true,
			},
		},
		{
			name: "period ends today",
			sub: &billing.OrgSubscription{
				PlanTier:          string(domain.PlanStarter),
				MonthlyUsageEmail: true,
				CurrentPeriodEnd:  &endsToday,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := newUsageReportCandidate("org-1", tc.sub, todayStart); ok {
				t.Fatal("expected ineligible usage report candidate")
			}
		})
	}
}
