package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Notion canonical values (source: "Strait — Pricing Model", page
// 3220936fb5548078bda2f5d83294ffa8). Any change to these numbers must
// land in Notion and in code in the same commit; this test is the gate.

type notionTier struct {
	planTier            domain.PlanTier
	priceMonthlyCents   int
	priceAnnualCents    int
	maxRunsPerMonth     int
	maxConcurrentRuns   int
	retentionDays       int
	maxEnvironments     int
	cronMinIntervalSec  int
	overagePerKMicrousd int64
}

func TestNotionLock_TierConstants(t *testing.T) {
	t.Parallel()

	want := []notionTier{
		{
			planTier:            domain.PlanFree,
			priceMonthlyCents:   0,
			priceAnnualCents:    0,
			maxRunsPerMonth:     5_000,
			maxConcurrentRuns:   3,
			retentionDays:       7,
			maxEnvironments:     1,
			cronMinIntervalSec:  300,
			overagePerKMicrousd: 500_000,
		},
		{
			planTier:            domain.PlanStarter,
			priceMonthlyCents:   1_900,
			priceAnnualCents:    18_000,
			maxRunsPerMonth:     50_000,
			maxConcurrentRuns:   15,
			retentionDays:       14,
			maxEnvironments:     1,
			cronMinIntervalSec:  60,
			overagePerKMicrousd: 400_000,
		},
		{
			planTier:            domain.PlanPro,
			priceMonthlyCents:   9_900,
			priceAnnualCents:    94_800,
			maxRunsPerMonth:     1_000_000,
			maxConcurrentRuns:   100,
			retentionDays:       30,
			maxEnvironments:     3,
			cronMinIntervalSec:  30,
			overagePerKMicrousd: 200_000,
		},
		{
			planTier:            domain.PlanScale,
			priceMonthlyCents:   29_900,
			priceAnnualCents:    286_800,
			maxRunsPerMonth:     5_000_000,
			maxConcurrentRuns:   300,
			retentionDays:       60,
			maxEnvironments:     10,
			cronMinIntervalSec:  1,
			overagePerKMicrousd: 60_000,
		},
		{
			planTier:            domain.PlanBusiness,
			priceMonthlyCents:   49_900,
			priceAnnualCents:    478_800,
			maxRunsPerMonth:     25_000_000,
			maxConcurrentRuns:   500,
			retentionDays:       90,
			maxEnvironments:     -1,
			cronMinIntervalSec:  0,
			overagePerKMicrousd: 30_000,
		},
		{
			planTier:            domain.PlanEnterprise,
			priceMonthlyCents:   0,
			priceAnnualCents:    0,
			maxRunsPerMonth:     -1,
			maxConcurrentRuns:   -1,
			retentionDays:       -1,
			maxEnvironments:     -1,
			cronMinIntervalSec:  0,
			overagePerKMicrousd: 30_000,
		},
	}

	for _, w := range want {
		got, ok := Plans[w.planTier]
		require.True(t, ok)
		assert.Equal(t, w.
			priceMonthlyCents, got.PriceMonthlyUsd,
		)
		assert.Equal(t, w.
			priceAnnualCents, got.PriceAnnualUsd,
		)
		assert.Equal(t, w.
			maxRunsPerMonth, got.MaxRunsPerMonth,
		)
		assert.Equal(t, w.
			maxConcurrentRuns, got.MaxConcurrentRuns,
		)
		assert.Equal(t, w.
			retentionDays, got.RetentionDays,
		)
		assert.Equal(t, w.
			maxEnvironments, got.MaxEnvironments,
		)
		assert.Equal(t, w.
			cronMinIntervalSec, got.CronMinIntervalSec,
		)
		assert.Equal(t, w.
			overagePerKMicrousd, got.OveragePerKMicrousd,
		)

	}
}

func TestNotionLock_SpendingCapsMicrousd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"Free", MaxSpendingFree, 50_000_000},
		{"Starter", MaxSpendingStarter, 100_000_000},
		{"Pro", MaxSpendingPro, 200_000_000},
		{"Scale", MaxSpendingScale, 500_000_000},
		{"Business", MaxSpendingBusiness, 1_500_000_000},
	}
	for _, c := range cases {
		assert.Equal(t, c.
			want, c.got)

	}
}

func TestNotionLock_AddonPrices(t *testing.T) {
	t.Parallel()

	want := map[AddonType]int{
		AddonConcurrency100:    2000, // $20
		AddonHistory30d:        4000, // $40
		AddonComplianceArchive: 0,    // roadmap, not sellable at launch
		AddonDedicatedWorkers:  0,    // roadmap, not sellable at launch
		AddonEnvironments5:     3000, // $30
	}
	for at, wantCents := range want {
		pack, ok := AddonPacks[at]
		require.True(t, ok)
		assert.Equal(t, wantCents,

			pack.PriceCents)

	}
}

func TestNotionLock_SLACreditBands(t *testing.T) {
	t.Parallel()
	require.Len(t, SLACreditTiers,

		3)

	want := []int{10, 25, 50}
	for i, w := range want {
		assert.Equal(t, w,

			SLACreditTiers[i].CreditPct,
		)

	}
}
