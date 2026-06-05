package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCurrentUsage_EnterpriseWithContract(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {
				OrgID:              "org-ent",
				PlanTier:           "enterprise",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
		enterpriseContracts: map[string]*EnterpriseContract{
			"org-ent": {
				OrgID:              "org-ent",
				EnterpriseTier:     EnterpriseTierStarter,
				OverageDiscountPct: EnterpriseStarterOverageDiscountPct,
				ContractEndDate:    now.AddDate(1, 0, 0),
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-ent": 500_000_000, // $500 spend
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-ent")
	require.NoError(t,
		err)
	assert.EqualValues(t, 500_000_000,

		resp.PeriodSpendMicro,
	)
	assert.Equal(t, string(EnterpriseTierStarter), resp.EnterpriseTier)
	assert.Equal(t, 10,
		resp.OverageDiscountPct,
	)
	assert.InDelta(t, 99.9,
		resp.SLAUptimePct, 1e-9,
	)
}

func TestGetCurrentUsage_EnterpriseNoContract(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {
				OrgID:              "org-ent",
				PlanTier:           "enterprise",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-ent")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		resp.OverageMicro,
	)
	assert.Empty(t, resp.EnterpriseTier)
}

func TestGetCurrentUsage_EnterpriseOverage_DiscountApplied(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {
				OrgID:              "org-ent",
				PlanTier:           "enterprise",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
		enterpriseContracts: map[string]*EnterpriseContract{
			"org-ent": {
				OrgID:              "org-ent",
				EnterpriseTier:     EnterpriseTierStarter,
				OverageDiscountPct: 10,
				ContractEndDate:    now.AddDate(1, 0, 0),
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-ent": 1_500_000_000, // $1,500
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-ent")
	require.NoError(t,
		err)

	// Launch billing has no spend-credit pool: the negotiated discount applies
	// to total overage spend.
	expectedOverage := int64(1_350_000_000)
	assert.Equal(t, expectedOverage,

		resp.
			OverageMicro,
	)
}

func TestGetCurrentUsage_NonEnterprise_NoEnterpriseFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {
				OrgID:              "org-pro",
				PlanTier:           "pro",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-pro")
	require.NoError(t,
		err)
	assert.Empty(t, resp.EnterpriseTier)
	assert.Equal(t, 0,
		resp.OverageDiscountPct,
	)
	assert.InDelta(t, 0,
		resp.SLAUptimePct, 1e-9,
	)
}

func TestGetCurrentUsage_EnterpriseContractEndDate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)
	contractEnd := now.AddDate(1, 0, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {
				OrgID:              "org-ent",
				PlanTier:           "enterprise",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
		enterpriseContracts: map[string]*EnterpriseContract{
			"org-ent": {
				OrgID:              "org-ent",
				EnterpriseTier:     EnterpriseTierGrowth,
				OverageDiscountPct: 15,
				ContractEndDate:    contractEnd,
			},
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-ent")
	require.NoError(t,
		err)

	expected := contractEnd.Format("2006-01-02")
	assert.Equal(t, expected,
		resp.
			ContractEndDate,
	)
	assert.InDelta(t, 99.95,
		resp.
			SLAUptimePct, 1e-9,
	)
}

func TestGetCurrentUsage_EnterpriseGrowthDiscount15Pct(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {
				OrgID:              "org-ent",
				PlanTier:           "enterprise",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
		enterpriseContracts: map[string]*EnterpriseContract{
			"org-ent": {
				OrgID:              "org-ent",
				EnterpriseTier:     EnterpriseTierGrowth,
				OverageDiscountPct: 15,
				ContractEndDate:    now.AddDate(1, 0, 0),
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-ent": 3_500_000_000, // $3,500
		},
	}

	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-ent")
	require.NoError(t,
		err)

	// Launch billing has no spend-credit pool: the negotiated discount applies
	// to total overage spend.
	expectedOverage := int64(2_975_000_000)
	assert.Equal(t, expectedOverage,

		resp.
			OverageMicro,
	)
}
