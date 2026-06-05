package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzEnterpriseTierValidation(f *testing.F) {
	f.Add("enterprise_starter")
	f.Add("enterprise_growth")
	f.Add("enterprise_large")
	f.Add("")
	f.Add("free")
	f.Add("enterprise")
	f.Add("ENTERPRISE_STARTER")

	f.Fuzz(func(t *testing.T, s string) {
		tier := EnterpriseTier(s)
		valid := IsValidEnterpriseTier(tier)

		// If valid, it must be one of the three known tiers.
		if valid {
			switch tier {
			case EnterpriseTierStarter, EnterpriseTierGrowth, EnterpriseTierLarge:
				// ok
			default:
				assert.Failf(t, "test failure", "IsValidEnterpriseTier(%q) = true but not a known tier", s)
			}
		}
	})
}

func FuzzApplyOverageDiscount(f *testing.F) {
	f.Add(int64(1000000), 10)
	f.Add(int64(0), 0)
	f.Add(int64(-1), 50)
	f.Add(int64(9223372036854775807), 99) // MaxInt64
	f.Add(int64(1), 100)

	f.Fuzz(func(t *testing.T, cost int64, discount int) {
		result := ApplyOverageDiscount(cost, discount)
		assert.GreaterOrEqual(t, result, int64(0))
		assert.False(t, cost <=
			0 && result != 0)
		assert.False(t, cost >
			0 && discount >= 100 && result != 0)
		assert.False(t, cost >
			0 && discount <= 0 && result != cost)

		// Result should never be negative.

		// If cost <= 0, result should be 0.

		// If discount >= 100, result should be 0 (for positive cost).

		// If discount <= 0 and cost > 0, result should be original cost.

	})
}

func FuzzEnterpriseTierForPrice(f *testing.F) {
	f.Add("")
	f.Add("price_123")
	f.Add("test_starter")
	f.Add("'; DROP TABLE--")

	f.Fuzz(func(t *testing.T, priceID string) {
		// Should never panic.
		tier, ok := EnterpriseTierForPrice(priceID)
		assert.False(t, ok &&
			!IsValidEnterpriseTier(tier))

	})
}
