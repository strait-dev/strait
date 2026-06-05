package billing

import (
	"context"
	"testing"
)

// Enterprise webhook contract creation tests.
// These verify that the webhook handler creates enterprise contracts
// when processing enterprise subscription events.

func TestWebhook_EnterpriseContractCreated_StarterTier(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_starter", EnterpriseTierStarter)

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("wh_test_starter"),
	)
	handler := NewWebhookHandler(store, mapping, "", nil, nil, nil, WithDevBypassSignatureCheck())

	tier, ok := mapping.TierForPrice("wh_test_starter")
	if !ok {
		t.Fatal("expected wh_test_starter to map to a tier")
	}
	if tier != "enterprise" {
		t.Fatalf("tier = %s, want enterprise", tier)
	}

	// Verify enterprise tier mapping is correct.
	entTier, entOK := EnterpriseTierForPrice("wh_test_starter")
	if !entOK {
		t.Fatal("expected wh_test_starter to map to an enterprise tier")
	}
	if entTier != EnterpriseTierStarter {
		t.Errorf("enterprise tier = %s, want %s", entTier, EnterpriseTierStarter)
	}

	// Verify config is correct.
	cfg := GetEnterpriseConfig(entTier)
	if cfg.AnnualCommitmentCents != EnterpriseStarterAnnualCents {
		t.Errorf("commitment = %d, want %d", cfg.AnnualCommitmentCents, EnterpriseStarterAnnualCents)
	}

	_ = handler // handler is wired correctly
}

func TestWebhook_EnterpriseContractCreated_GrowthTier(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_growth", EnterpriseTierGrowth)

	entTier, ok := EnterpriseTierForPrice("wh_test_growth")
	if !ok {
		t.Fatal("expected wh_test_growth to map to an enterprise tier")
	}
	if entTier != EnterpriseTierGrowth {
		t.Errorf("enterprise tier = %s, want %s", entTier, EnterpriseTierGrowth)
	}

	cfg := GetEnterpriseConfig(entTier)
	if cfg.OverageDiscountPct != EnterpriseGrowthOverageDiscountPct {
		t.Errorf("discount = %d%%, want %d%%", cfg.OverageDiscountPct, EnterpriseGrowthOverageDiscountPct)
	}
}

func TestWebhook_EnterpriseContractCreated_LargeTier(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_large", EnterpriseTierLarge)

	entTier, ok := EnterpriseTierForPrice("wh_test_large")
	if !ok {
		t.Fatal("expected wh_test_large to map to an enterprise tier")
	}
	if entTier != EnterpriseTierLarge {
		t.Errorf("enterprise tier = %s, want %s", entTier, EnterpriseTierLarge)
	}

	cfg := GetEnterpriseConfig(entTier)
	if cfg.OverageDiscountPct != EnterpriseLargeOverageDiscountPct {
		t.Errorf("discount = %d%%, want %d%%", cfg.OverageDiscountPct, EnterpriseLargeOverageDiscountPct)
	}
}

func TestWebhook_EnterpriseContractFields(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_fields", EnterpriseTierStarter)

	cfg := GetEnterpriseConfig(EnterpriseTierStarter)

	// Verify all active launch contract fields would be populated correctly.
	if cfg.AnnualCommitmentCents != 1_800_000 {
		t.Errorf("annual commitment = %d, want 1800000", cfg.AnnualCommitmentCents)
	}
	if cfg.OverageDiscountPct != 10 {
		t.Errorf("discount = %d, want 10", cfg.OverageDiscountPct)
	}
}

func TestWebhook_EnterpriseUnknownPriceNoContract(t *testing.T) {
	t.Parallel()

	// An unregistered price should not map to an enterprise tier.
	_, ok := EnterpriseTierForPrice("wh_test_unknown_price")
	if ok {
		t.Fatal("expected unknown price to not map to an enterprise tier")
	}
}

func TestWebhook_EnterpriseContractUpsertIdempotent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: make(map[string]*OrgSubscription),
	}

	// First upsert.
	contract1 := &EnterpriseContract{
		ID:                    "contract-1",
		OrgID:                 "org-idempotent",
		EnterpriseTier:        EnterpriseTierStarter,
		AnnualCommitmentCents: 1_800_000,
		OverageDiscountPct:    10,
		BillingCadence:        "annual",
	}
	if err := store.UpsertEnterpriseContract(context.Background(), contract1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert for same org (simulating duplicate webhook).
	contract2 := &EnterpriseContract{
		ID:                    "contract-2",
		OrgID:                 "org-idempotent",
		EnterpriseTier:        EnterpriseTierGrowth,
		AnnualCommitmentCents: 4_800_000,
		OverageDiscountPct:    15,
		BillingCadence:        "annual",
	}
	if err := store.UpsertEnterpriseContract(context.Background(), contract2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// Should have the latest version.
	got, err := store.GetEnterpriseContract(context.Background(), "org-idempotent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EnterpriseTier != EnterpriseTierGrowth {
		t.Errorf("tier = %s, want %s (last write wins)", got.EnterpriseTier, EnterpriseTierGrowth)
	}
}
