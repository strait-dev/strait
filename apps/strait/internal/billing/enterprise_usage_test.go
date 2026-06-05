package billing

import (
	"context"
	"testing"
	"time"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.PeriodSpendMicro != 500_000_000 {
		t.Errorf("PeriodSpendMicro = %d, want 500000000", resp.PeriodSpendMicro)
	}

	if resp.EnterpriseTier != string(EnterpriseTierStarter) {
		t.Errorf("EnterpriseTier = %q, want %q", resp.EnterpriseTier, EnterpriseTierStarter)
	}

	if resp.OverageDiscountPct != 10 {
		t.Errorf("OverageDiscountPct = %d, want 10", resp.OverageDiscountPct)
	}

	if resp.SLAUptimePct != 99.9 {
		t.Errorf("SLAUptimePct = %.2f, want 99.9", resp.SLAUptimePct)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.OverageMicro != 0 {
		t.Errorf("OverageMicro = %d, want 0 (no spend)", resp.OverageMicro)
	}

	if resp.EnterpriseTier != "" {
		t.Errorf("EnterpriseTier = %q, want empty (no contract)", resp.EnterpriseTier)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Launch billing has no spend-credit pool: the negotiated discount applies
	// to total overage spend.
	expectedOverage := int64(1_350_000_000)
	if resp.OverageMicro != expectedOverage {
		t.Errorf("OverageMicro = %d, want %d (10%% discount on total overage spend)", resp.OverageMicro, expectedOverage)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.EnterpriseTier != "" {
		t.Errorf("EnterpriseTier = %q, want empty for non-enterprise", resp.EnterpriseTier)
	}
	if resp.OverageDiscountPct != 0 {
		t.Errorf("OverageDiscountPct = %d, want 0", resp.OverageDiscountPct)
	}
	if resp.SLAUptimePct != 0 {
		t.Errorf("SLAUptimePct = %.2f, want 0", resp.SLAUptimePct)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := contractEnd.Format("2006-01-02")
	if resp.ContractEndDate != expected {
		t.Errorf("ContractEndDate = %q, want %q", resp.ContractEndDate, expected)
	}

	if resp.SLAUptimePct != 99.95 {
		t.Errorf("SLAUptimePct = %.2f, want 99.95 (growth tier)", resp.SLAUptimePct)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Launch billing has no spend-credit pool: the negotiated discount applies
	// to total overage spend.
	expectedOverage := int64(2_975_000_000)
	if resp.OverageMicro != expectedOverage {
		t.Errorf("OverageMicro = %d, want %d (15%% discount on total overage spend)", resp.OverageMicro, expectedOverage)
	}
}
