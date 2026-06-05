package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithBusinessPrices_Resolves(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("biz-month-id", "biz-year-id"),
	)

	cases := []struct {
		name  string
		price string
	}{
		{"monthly", "biz-month-id"},
		{"yearly", "biz-year-id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tier, ok := m.TierForPrice(c.price)
			require.True(t, ok)
			assert.Equal(t, domain.
				PlanBusiness, tier)

		})
	}
}

func TestWithBusinessPrices_UnknownFallsThrough(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("biz-month-id", "biz-year-id"),
	)
	tier, ok := m.TierForPrice("not-a-business-price")
	assert.False(t, ok)
	assert.Equal(t, domain.
		PlanFree, tier)

}

func TestWithBusinessPrices_EmptyIDsIgnored(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("", ""),
	)
	assert.EqualValues(t, 0,

		m.PriceCount())

}

func TestWithBusinessFlatPrice_Resolves(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessFlatPrice("biz-flat"),
	)
	tier, ok := m.TierForPrice("biz-flat")
	assert.False(t, !ok ||
		tier != domain.PlanBusiness,
	)

}

// CatalogResolver already publishes the Business lookup keys; this test
// pins that regression so a future refactor of the resolver does not
// silently strip them.
func TestCatalogResolver_BusinessLookupKeysRegistered(t *testing.T) {
	t.Parallel()

	r := NewCatalogResolver()
	for _, key := range []string{"strait_business_monthly", "strait_business_annual"} {
		got, ok := r.TierForLookupKey(key)
		require.True(t, ok)
		assert.Equal(t, domain.
			PlanBusiness, got)

	}
}
