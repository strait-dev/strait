package billing

import (
	"testing"

	"strait/internal/domain"
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
			if !ok {
				t.Fatalf("TierForPrice(%q) ok=false, want true", c.price)
			}
			if tier != domain.PlanBusiness {
				t.Errorf("TierForPrice(%q) = %q, want %q", c.price, tier, domain.PlanBusiness)
			}
		})
	}
}

func TestWithBusinessPrices_UnknownFallsThrough(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("biz-month-id", "biz-year-id"),
	)
	tier, ok := m.TierForPrice("not-a-business-price")
	if ok {
		t.Errorf("unknown price ok = true, want false")
	}
	if tier != domain.PlanFree {
		t.Errorf("unknown price tier = %q, want %q", tier, domain.PlanFree)
	}
}

func TestWithBusinessPrices_EmptyIDsIgnored(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessPrices("", ""),
	)
	if m.PriceCount() != 0 {
		t.Errorf("PriceCount() = %d, want 0 (empty IDs must not register)", m.PriceCount())
	}
}

func TestWithBusinessFlatPrice_Resolves(t *testing.T) {
	t.Parallel()

	m := NewStripeMappingFromOptions(
		WithBusinessFlatPrice("biz-flat"),
	)
	tier, ok := m.TierForPrice("biz-flat")
	if !ok || tier != domain.PlanBusiness {
		t.Errorf("WithBusinessFlatPrice resolution = (%q, %v), want (Business, true)", tier, ok)
	}
}

// CatalogResolver already publishes the Business lookup keys; this test
// pins that regression so a future refactor of the resolver does not
// silently strip them.
func TestCatalogResolver_BusinessLookupKeysRegistered(t *testing.T) {
	t.Parallel()

	r := NewCatalogResolver()
	for _, key := range []string{"strait_business_monthly", "strait_business_annual"} {
		got, ok := r.TierForLookupKey(key)
		if !ok {
			t.Fatalf("lookup key %q missing from CatalogResolver", key)
		}
		if got != domain.PlanBusiness {
			t.Errorf("CatalogResolver[%q] = %q, want %q", key, got, domain.PlanBusiness)
		}
	}
}
