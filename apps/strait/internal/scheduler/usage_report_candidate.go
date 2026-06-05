package scheduler

import (
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

type usageReportCandidate struct {
	orgID     string
	sub       *billing.OrgSubscription
	periodEnd time.Time
}

func newUsageReportCandidate(
	orgID string,
	sub *billing.OrgSubscription,
	todayStart time.Time,
) (usageReportCandidate, bool) {
	if sub == nil || !sub.MonthlyUsageEmail || sub.CurrentPeriodEnd == nil {
		return usageReportCandidate{}, false
	}

	tier := billing.GetPlanLimits(domain.PlanTier(sub.PlanTier))
	if tier.PriceMonthlyUsd == 0 && tier.PriceAnnualUsd == 0 {
		return usageReportCandidate{}, false
	}

	periodEnd := sub.CurrentPeriodEnd.UTC().Truncate(24 * time.Hour)
	if !periodEnd.Before(todayStart) {
		return usageReportCandidate{}, false
	}

	return usageReportCandidate{
		orgID:     orgID,
		sub:       sub,
		periodEnd: periodEnd,
	}, true
}
