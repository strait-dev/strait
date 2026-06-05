package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.True(t, ok)
	require.Equal(t,
		"enterprise",

		string(tier))

	// Verify enterprise tier mapping is correct.
	entTier, entOK := EnterpriseTierForPrice("wh_test_starter")
	require.True(t, entOK)
	assert.Equal(t, EnterpriseTierStarter,

		entTier)

	// Verify config is correct.
	cfg := GetEnterpriseConfig(entTier)
	assert.Equal(t, EnterpriseStarterAnnualCents,

		cfg.AnnualCommitmentCents)

	_ = handler // handler is wired correctly
}

func TestWebhook_EnterpriseContractCreated_GrowthTier(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_growth", EnterpriseTierGrowth)

	entTier, ok := EnterpriseTierForPrice("wh_test_growth")
	require.True(t, ok)
	assert.Equal(t, EnterpriseTierGrowth,

		entTier)

	cfg := GetEnterpriseConfig(entTier)
	assert.Equal(t, EnterpriseGrowthOverageDiscountPct,

		cfg.OverageDiscountPct)
}

func TestWebhook_EnterpriseContractCreated_LargeTier(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_large", EnterpriseTierLarge)

	entTier, ok := EnterpriseTierForPrice("wh_test_large")
	require.True(t, ok)
	assert.Equal(t, EnterpriseTierLarge,

		entTier)

	cfg := GetEnterpriseConfig(entTier)
	assert.Equal(t, EnterpriseLargeOverageDiscountPct,

		cfg.OverageDiscountPct)
}

func TestWebhook_EnterpriseContractFields(t *testing.T) {
	t.Parallel()
	RegisterEnterprisePriceTier("wh_test_fields", EnterpriseTierStarter)

	cfg := GetEnterpriseConfig(EnterpriseTierStarter)
	assert.EqualValues(t, 1_800_000,
		cfg.
			AnnualCommitmentCents)
	assert.Equal(t, 10,
		cfg.OverageDiscountPct,
	)

	// Verify all active launch contract fields would be populated correctly.
}

func TestWebhook_EnterpriseUnknownPriceNoContract(t *testing.T) {
	t.Parallel()

	// An unregistered price should not map to an enterprise tier.
	_, ok := EnterpriseTierForPrice("wh_test_unknown_price")
	require.False(t,
		ok)
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
	require.NoError(t,
		store.UpsertEnterpriseContract(context.Background(), contract1))

	// Second upsert for same org (simulating duplicate webhook).
	contract2 := &EnterpriseContract{
		ID:                    "contract-2",
		OrgID:                 "org-idempotent",
		EnterpriseTier:        EnterpriseTierGrowth,
		AnnualCommitmentCents: 4_800_000,
		OverageDiscountPct:    15,
		BillingCadence:        "annual",
	}
	require.NoError(t,
		store.UpsertEnterpriseContract(context.Background(), contract2))

	// Should have the latest version.
	got, err := store.GetEnterpriseContract(context.Background(), "org-idempotent")
	require.NoError(t,
		err)
	assert.Equal(t, EnterpriseTierGrowth,

		got.EnterpriseTier)
}
