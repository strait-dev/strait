package billing

import (
	"time"

	"strait/internal/domain"
)

// SpendingLimitPresets defines the available preset spending limit options in micro-USD.
var SpendingLimitPresets = []int64{
	0,          // $0 - hard cap, no overage
	25000000,   // $25
	50000000,   // $50
	100000000,  // $100
	250000000,  // $250
	500000000,  // $500
	2000000000, // $2,000
}

// MaxSpendingLimit returns the maximum allowed spending limit for a plan tier.
func MaxSpendingLimit(tier domain.PlanTier) int64 {
	switch tier {
	case domain.PlanStarter:
		return MaxSpendingStarter
	case domain.PlanPro:
		return MaxSpendingPro
	case domain.PlanEnterprise:
		return -1 // custom
	default:
		return 0 // free: no spending limit
	}
}

func usagePeriodWindow(now time.Time, tier domain.PlanTier, sub *OrgSubscription) (time.Time, time.Time) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)

	if tier == domain.PlanFree || sub == nil {
		return monthStart, monthEnd
	}

	start := monthStart
	end := monthEnd
	if sub.CurrentPeriodStart != nil {
		start = *sub.CurrentPeriodStart
	}
	if sub.CurrentPeriodEnd != nil {
		end = *sub.CurrentPeriodEnd
	}
	return start, end
}

func computeOverageSpend(periodSpend, includedCredit int64) int64 {
	return max(periodSpend-includedCredit, 0)
}

func isOverageLimitReached(limitMicrousd, overageSpendMicrousd int64) bool {
	switch {
	case limitMicrousd < 0:
		return false
	case limitMicrousd == 0:
		return overageSpendMicrousd > 0
	default:
		return overageSpendMicrousd >= limitMicrousd
	}
}

// SpendingLimitResponse is the API response for spending limit queries.
type SpendingLimitResponse struct {
	OrgID             string  `json:"org_id"`
	PlanTier          string  `json:"plan_tier"`
	SpendingLimitUsd  float64 `json:"spending_limit_usd"`
	LimitAction       string  `json:"limit_action"`
	CurrentSpendUsd   float64 `json:"current_spend_usd"`
	IncludedCreditUsd float64 `json:"included_credit_usd"`
	OverageSpendUsd   float64 `json:"overage_spend_usd"`
	IsHardCapped      bool    `json:"is_hard_capped"`
}
