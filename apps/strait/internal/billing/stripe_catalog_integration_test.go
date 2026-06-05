//go:build integration

package billing_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/price"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestStripeCatalog_SandboxShape verifies that every canonical lookup key in
// PlanCatalogs resolves to a Stripe Price with the shape we depend on:
//
//   - Active (we never want to silently bind to an archived price)
//   - Currency=usd (the meter and overage math assume cents)
//   - Recurring with interval=month/year (or unset for non-recurring overage)
//   - For overage keys: UsageType=metered, Meter is non-empty
//
// The test is gated by STRAIT_STRIPE_INTEGRATION=1 and reads STRIPE_SECRET_KEY
// from the environment so CI doesn't accidentally hit the live Stripe API.
// Operators run it manually after a sandbox catalog migration to confirm the
// account state matches the Notion-canonical model encoded in catalog.go.
func TestStripeCatalog_SandboxShape(t *testing.T) {
	if os.Getenv("STRAIT_STRIPE_INTEGRATION") != "1" {
		t.Skip("set STRAIT_STRIPE_INTEGRATION=1 to run; requires STRIPE_SECRET_KEY")
	}
	secret := os.Getenv("STRIPE_SECRET_KEY")
	require.NotEqual(t, "", secret)

	stripe.Key = secret

	type lookupCase struct {
		key      string
		tier     domain.PlanTier
		flavor   string // "monthly" | "annual" | "overage"
		expected expectedShape
	}

	expectedFor := func(flavor string) expectedShape {
		switch flavor {
		case "monthly":
			return expectedShape{
				active: true, currency: "usd",
				recurring: true, interval: stripe.PriceRecurringIntervalMonth,
				metered: false,
			}
		case "annual":
			return expectedShape{
				active: true, currency: "usd",
				recurring: true, interval: stripe.PriceRecurringIntervalYear,
				metered: false,
			}
		case "overage":
			return expectedShape{
				active: true, currency: "usd",
				recurring: true, interval: stripe.PriceRecurringIntervalMonth,
				metered: true,
			}
		}
		return expectedShape{}
	}

	var cases []lookupCase
	for tier, cat := range billing.PlanCatalogs {
		if cat.LookupKeyMonthly != "" {
			cases = append(cases, lookupCase{cat.LookupKeyMonthly, tier, "monthly", expectedFor("monthly")})
		}
		if cat.LookupKeyAnnual != "" {
			cases = append(cases, lookupCase{cat.LookupKeyAnnual, tier, "annual", expectedFor("annual")})
		}
		if cat.LookupKeyOverage != "" {
			cases = append(cases, lookupCase{cat.LookupKeyOverage, tier, "overage", expectedFor("overage")})
		}
	}
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			params := &stripe.PriceListParams{LookupKeys: []*string{&tc.key}}
			params.Limit = stripe.Int64(2)

			iter := price.List(params)
			seen := 0
			for iter.Next() {
				seen++
				p := iter.Price()
				if seen > 1 {
					assert.Failf(t, "test failure",

						"lookup_key %q matched more than one price (got %s)", tc.key, p.ID)
					continue
				}
				assertShape(t, tc.key, tc.flavor, p, tc.expected)
			}
			require.NoError(t, iter.Err())
			assert.NotEqual(t, 0, seen)

		})
	}
}

type expectedShape struct {
	active    bool
	currency  string
	recurring bool
	interval  stripe.PriceRecurringInterval
	metered   bool
}

func assertShape(t *testing.T, key, flavor string, p *stripe.Price, want expectedShape) {
	t.Helper()
	assert.Equal(t, want.active,

		p.Active,
	)
	assert.Equal(t, want.currency,

		string(p.Currency))

	if want.recurring {
		assert.NotNil(t, p.Recurring)
		assert.Equal(t, want.interval,

			p.Recurring.
				Interval)
		assert.False(t, want.metered &&
			p.Recurring.
				UsageType !=
				stripe.PriceRecurringUsageTypeMetered,
		)
		assert.False(t, want.metered &&
			p.Recurring.
				Meter ==

				"")

	}
}
