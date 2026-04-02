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
				OrgID:                  "org-ent",
				EnterpriseTier:         EnterpriseTierStarter,
				IncludedCreditMicrousd: EnterpriseStarterCreditMicrousd,
				ComputeDiscountPct:     EnterpriseStarterDiscountPct,
				ContractEndDate:        now.AddDate(1, 0, 0),
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

	// Credit pool should come from the contract.
	if resp.IncludedCreditMicro != EnterpriseStarterCreditMicrousd {
		t.Errorf("IncludedCreditMicro = %d, want %d", resp.IncludedCreditMicro, EnterpriseStarterCreditMicrousd)
	}

	if resp.EnterpriseTier != string(EnterpriseTierStarter) {
		t.Errorf("EnterpriseTier = %q, want %q", resp.EnterpriseTier, EnterpriseTierStarter)
	}

	if resp.ComputeDiscountPct != 10 {
		t.Errorf("ComputeDiscountPct = %d, want 10", resp.ComputeDiscountPct)
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

	if resp.IncludedCreditMicro != 0 {
		t.Errorf("IncludedCreditMicro = %d, want 0 (no contract)", resp.IncludedCreditMicro)
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
				OrgID:                  "org-ent",
				EnterpriseTier:         EnterpriseTierStarter,
				IncludedCreditMicrousd: 1_000_000_000, // $1,000
				ComputeDiscountPct:     10,
				ContractEndDate:        now.AddDate(1, 0, 0),
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

	// Raw overage: $1,500 - $1,000 = $500 = 500_000_000 micro-USD.
	// After 10% discount: $500 * 0.9 = $450 = 450_000_000 micro-USD.
	expectedOverage := int64(450_000_000)
	if resp.OverageMicro != expectedOverage {
		t.Errorf("OverageMicro = %d, want %d (10%% discount on $500 overage)", resp.OverageMicro, expectedOverage)
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
	if resp.ComputeDiscountPct != 0 {
		t.Errorf("ComputeDiscountPct = %d, want 0", resp.ComputeDiscountPct)
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
				OrgID:                  "org-ent",
				EnterpriseTier:         EnterpriseTierGrowth,
				IncludedCreditMicrousd: EnterpriseGrowthCreditMicrousd,
				ComputeDiscountPct:     15,
				ContractEndDate:        contractEnd,
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
				OrgID:                  "org-ent",
				EnterpriseTier:         EnterpriseTierGrowth,
				IncludedCreditMicrousd: 2_500_000_000, // $2,500
				ComputeDiscountPct:     15,
				ContractEndDate:        now.AddDate(1, 0, 0),
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

	// Raw overage: $3,500 - $2,500 = $1,000 = 1_000_000_000.
	// After 15% discount: $1,000 * 0.85 = $850 = 850_000_000.
	expectedOverage := int64(850_000_000)
	if resp.OverageMicro != expectedOverage {
		t.Errorf("OverageMicro = %d, want %d (15%% discount on $1000 overage)", resp.OverageMicro, expectedOverage)
	}
}
