package billing

import (
	"testing"

	"strait/internal/domain"
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
		if !ok {
			t.Fatalf("Plans[%q] missing", w.planTier)
		}
		if got.PriceMonthlyUsd != w.priceMonthlyCents {
			t.Errorf("%s PriceMonthlyUsd = %d, want %d", w.planTier, got.PriceMonthlyUsd, w.priceMonthlyCents)
		}
		if got.PriceAnnualUsd != w.priceAnnualCents {
			t.Errorf("%s PriceAnnualUsd = %d, want %d", w.planTier, got.PriceAnnualUsd, w.priceAnnualCents)
		}
		if got.MaxRunsPerMonth != w.maxRunsPerMonth {
			t.Errorf("%s MaxRunsPerMonth = %d, want %d", w.planTier, got.MaxRunsPerMonth, w.maxRunsPerMonth)
		}
		if got.MaxConcurrentRuns != w.maxConcurrentRuns {
			t.Errorf("%s MaxConcurrentRuns = %d, want %d", w.planTier, got.MaxConcurrentRuns, w.maxConcurrentRuns)
		}
		if got.RetentionDays != w.retentionDays {
			t.Errorf("%s RetentionDays = %d, want %d", w.planTier, got.RetentionDays, w.retentionDays)
		}
		if got.MaxEnvironments != w.maxEnvironments {
			t.Errorf("%s MaxEnvironments = %d, want %d", w.planTier, got.MaxEnvironments, w.maxEnvironments)
		}
		if got.CronMinIntervalSec != w.cronMinIntervalSec {
			t.Errorf("%s CronMinIntervalSec = %d, want %d", w.planTier, got.CronMinIntervalSec, w.cronMinIntervalSec)
		}
		if got.OveragePerKMicrousd != w.overagePerKMicrousd {
			t.Errorf("%s OveragePerKMicrousd = %d, want %d", w.planTier, got.OveragePerKMicrousd, w.overagePerKMicrousd)
		}
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
		if c.got != c.want {
			t.Errorf("MaxSpending%s = %d, want %d", c.name, c.got, c.want)
		}
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
		if !ok {
			t.Fatalf("AddonPacks[%q] missing", at)
		}
		if pack.PriceCents != wantCents {
			t.Errorf("AddonPacks[%q].PriceCents = %d, want %d", at, pack.PriceCents, wantCents)
		}
	}
}

func TestNotionLock_SLACreditBands(t *testing.T) {
	t.Parallel()

	if len(SLACreditTiers) != 3 {
		t.Fatalf("len(SLACreditTiers) = %d, want 3 (Notion canonical 10/25/50)", len(SLACreditTiers))
	}
	want := []int{10, 25, 50}
	for i, w := range want {
		if SLACreditTiers[i].CreditPct != w {
			t.Errorf("SLACreditTiers[%d].CreditPct = %d, want %d", i, SLACreditTiers[i].CreditPct, w)
		}
	}
}
