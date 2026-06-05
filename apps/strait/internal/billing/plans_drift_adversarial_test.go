package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// Cheaper-with-scale / pay-more-get-more invariants. If any of these flip
// in the future, the change is either a deliberate pricing decision (and
// the test must be updated alongside the Notion canonical doc) or a bug.

var paidTierOrder = []domain.PlanTier{
	domain.PlanStarter,
	domain.PlanPro,
	domain.PlanScale,
	domain.PlanBusiness,
}

func TestDrift_AnnualDiscountRange(t *testing.T) {
	t.Parallel()

	// Annual rate must be cheaper than 12 * monthly, but not more than 25%
	// off (sanity floor against accidental zero-price annual plans).
	for _, tier := range paidTierOrder {
		p := Plans[tier]
		monthly12 := int64(p.PriceMonthlyUsd) * 12
		annual := int64(p.PriceAnnualUsd)
		assert.False(t, annual >=
			monthly12)
		assert.GreaterOrEqual(t, annual*100, monthly12*
			75)

	}
}

func TestDrift_StrictlyMonotonicPaidTiers(t *testing.T) {
	t.Parallel()

	type metric struct {
		name string
		get  func(OrgPlanLimits) int64
		// increasing == true: value must strictly grow Starter→Business.
		// increasing == false: value must strictly shrink (cheaper overage).
		increasing bool
	}
	metrics := []metric{
		{"MaxRunsPerMonth", func(p OrgPlanLimits) int64 { return int64(p.MaxRunsPerMonth) }, true},
		{"MaxConcurrentRuns", func(p OrgPlanLimits) int64 { return int64(p.MaxConcurrentRuns) }, true},
		{"RetentionDays", func(p OrgPlanLimits) int64 { return int64(p.RetentionDays) }, true},
		{"OveragePerKMicrousd", func(p OrgPlanLimits) int64 { return p.OveragePerKMicrousd }, false},
	}
	spending := []int64{MaxSpendingStarter, MaxSpendingPro, MaxSpendingScale, MaxSpendingBusiness}
	for i := 1; i < len(spending); i++ {
		assert.False(t, spending[i] <= spending[i-1])

	}
	for _, m := range metrics {
		var prev int64
		for i, tier := range paidTierOrder {
			cur := m.get(Plans[tier])
			// Business has -1 (unlimited) for several fields; treat as "max".
			if i == 0 {
				prev = cur
				continue
			}
			if m.increasing {
				curEff := cur
				prevEff := prev
				if curEff == -1 {
					curEff = 1<<62 - 1
				}
				if prevEff == -1 {
					prevEff = 1<<62 - 1
				}
				assert.False(t, curEff <=
					prevEff)

			} else if cur >= prev {
				assert.Failf(t, "test failure",

					"%s must strictly decrease %s -> %s: %d >= %d",
					m.name, paidTierOrder[i-1], tier, cur, prev)
			}
			prev = cur
		}
	}
}

func TestDrift_EnterpriseUnlimited(t *testing.T) {
	t.Parallel()

	p := Plans[domain.PlanEnterprise]
	checks := []struct {
		name string
		got  int
	}{
		{"MaxRunsPerMonth", p.MaxRunsPerMonth},
		{"MaxConcurrentRuns", p.MaxConcurrentRuns},
		{"RetentionDays", p.RetentionDays},
		{"MaxEnvironments", p.MaxEnvironments},
		{"MaxProjectsPerOrg", p.MaxProjectsPerOrg},
		{"MaxMembersPerOrg", p.MaxMembersPerOrg},
		{"MaxOrgsPerUser", p.MaxOrgsPerUser},
		{"MaxScheduledJobs", p.MaxScheduledJobs},
		{"MaxLogDrainsPerOrg", p.MaxLogDrainsPerOrg},
		{"MaxWebhookEndpoints", p.MaxWebhookEndpoints},
		{"APIRateLimit", p.APIRateLimit},
	}
	for _, c := range checks {
		assert.EqualValues(t, -1,
			c.got)

	}
}

func TestDrift_SLACreditBoundaries(t *testing.T) {
	t.Parallel()

	const target = EnterpriseStarterSLAPct // 99.9
	cases := []struct {
		uptime float64
		want   int
	}{
		{99.95, 0},  // above target
		{99.9, 0},   // at target
		{99.89, 10}, // just below target → first band
		{99.0, 10},  // at [99.0, 99.9) band lower bound
		{98.99, 25}, // just below 99.0 → middle band
		{95.0, 25},  // at [95.0, 99.0) lower bound
		{94.99, 50}, // just below 95.0 → bottom band (collapsed)
		{50.0, 50},  // deep in bottom band
		{0.0, 50},   // floor of bottom band
	}
	for _, c := range cases {
		got := CalculateSLACredit(c.uptime, target)
		assert.Equal(t, c.
			want, got)

	}
}
