//go:build integration

package billing_test

import (
	"os"
	"testing"

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
	if secret == "" {
		t.Fatal("STRIPE_SECRET_KEY must be set when STRAIT_STRIPE_INTEGRATION=1")
	}
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

	if len(cases) == 0 {
		t.Fatal("PlanCatalogs has no lookup keys to verify")
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			lookupKeyCopy := tc.key
			params := &stripe.PriceListParams{LookupKeys: []*string{&lookupKeyCopy}}
			params.Limit = stripe.Int64(2)

			iter := price.List(params)
			seen := 0
			for iter.Next() {
				seen++
				p := iter.Price()
				if seen > 1 {
					t.Errorf("lookup_key %q matched more than one price (got %s)", tc.key, p.ID)
					continue
				}
				assertShape(t, tc.key, tc.flavor, p, tc.expected)
			}
			if err := iter.Err(); err != nil {
				t.Fatalf("listing prices for %q: %v", tc.key, err)
			}
			if seen == 0 {
				t.Errorf("lookup_key %q is in catalog.go but not in Stripe sandbox — catalog drift", tc.key)
			}
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

	if p.Active != want.active {
		t.Errorf("%s (%s): Active=%v, want %v", key, flavor, p.Active, want.active)
	}
	if string(p.Currency) != want.currency {
		t.Errorf("%s (%s): Currency=%q, want %q", key, flavor, p.Currency, want.currency)
	}
	if want.recurring {
		if p.Recurring == nil {
			t.Errorf("%s (%s): expected recurring config, got nil", key, flavor)
			return
		}
		if p.Recurring.Interval != want.interval {
			t.Errorf("%s (%s): Recurring.Interval=%q, want %q",
				key, flavor, p.Recurring.Interval, want.interval)
		}
		if want.metered && p.Recurring.UsageType != stripe.PriceRecurringUsageTypeMetered {
			t.Errorf("%s (%s): UsageType=%q, want metered", key, flavor, p.Recurring.UsageType)
		}
		if want.metered && p.Recurring.Meter == "" {
			t.Errorf("%s (%s): metered price has empty Meter id", key, flavor)
		}
	}
}
