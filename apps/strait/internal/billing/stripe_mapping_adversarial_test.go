package billing

import (
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStripeMapping_EmptyProductIDs verifies that empty string product IDs are silently skipped.
func TestStripeMapping_EmptyProductIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("", "", "", "")
	require.Equal(t, 0, m.PriceCount())

	tier, ok := m.TierForPrice("")
	require.False(t,
		ok)
	require.Equal(t,
		domain.PlanFree,

		tier)
}

// TestStripeMapping_DuplicateProductIDs verifies that the same product ID used
// for multiple tiers results in last-write-wins behavior.
func TestStripeMapping_DuplicateProductIDs(t *testing.T) {
	t.Parallel()

	// "dup-id" is used for both starter monthly and pro monthly.
	// Since pro monthly is assigned after starter monthly, it wins.
	m := NewStripeMapping("dup-id", "starter-yearly", "dup-id", "pro-yearly")

	tier, ok := m.TierForPrice("dup-id")
	require.True(t, ok)
	require.Equal(t,
		domain.PlanPro,

		tier)
}

// TestTierForPrice_UnknownProduct verifies that an unknown product ID returns PlanFree and false.
func TestTierForPrice_UnknownProduct(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("nonexistent-product-id")
	require.False(t,
		ok)
	require.Equal(t,
		domain.PlanFree,

		tier)
}

// TestTierForPrice_EmptyString verifies that an empty string product ID returns PlanFree and false.
func TestTierForPrice_EmptyString(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("")
	require.False(t,
		ok)
	require.Equal(t,
		domain.PlanFree,

		tier)
}

// TestTierForPrice_NullBytes verifies that null bytes in a product ID do not cause panics.
func TestTierForPrice_NullBytes(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("product\x00id")
	require.False(t,
		ok)
	require.Equal(t,
		domain.PlanFree,

		tier)
}

// TestTierForPrice_AllTiers verifies that each tier product resolves correctly.
func TestTierForPrice_AllTiers(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	cases := []struct {
		productID string
		wantTier  domain.PlanTier
	}{
		{"starter-m", domain.PlanStarter},
		{"starter-y", domain.PlanStarter},
		{"pro-m", domain.PlanPro},
		{"pro-y", domain.PlanPro},
	}

	for _, tc := range cases {
		tier, ok := m.TierForPrice(tc.productID)
		if !ok {
			assert.Failf(t, "test failure",

				"expected true for product %q", tc.productID)
			continue
		}
		assert.Equal(t, tc.
			wantTier,

			tier)
	}
}

// FuzzTierForPrice fuzzes product ID strings to ensure no panics.
func FuzzTierForPrice(f *testing.F) {
	f.Add("starter-m")
	f.Add("")
	f.Add("product\x00id")
	f.Add(strings.Repeat("a", 10000))

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	f.Fuzz(func(t *testing.T, productID string) {
		tier, ok := m.TierForPrice(productID)
		assert.False(t, !ok && tier !=
			domain.PlanFree)
		assert.False(t, ok &&
			tier !=

				domain.PlanStarter && tier !=
			domain.PlanPro)
	})
}

// TestStripeMapping_CaseSensitivity verifies that product ID lookups are case-sensitive.
func TestStripeMapping_CaseSensitivity(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("Starter-M", "starter-y", "pro-m", "pro-y")

	// Exact case should match.
	tier, ok := m.TierForPrice("Starter-M")
	require.True(t, ok)
	require.Equal(t,
		domain.PlanStarter,

		tier)

	// Different case should not match.
	_, ok = m.TierForPrice("starter-m")
	require.False(t,
		ok)

	_, ok = m.TierForPrice("STARTER-M")
	require.False(t,
		ok)
}
