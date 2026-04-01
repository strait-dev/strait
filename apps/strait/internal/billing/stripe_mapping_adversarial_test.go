package billing

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestStripeMapping_EmptyProductIDs verifies that empty string product IDs are silently skipped.
func TestStripeMapping_EmptyProductIDs(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("", "", "", "")

	if m.PriceCount() != 0 {
		t.Fatalf("expected empty mapping when all product IDs are empty, got %d entries", m.PriceCount())
	}

	tier, ok := m.TierForPrice("")
	if ok {
		t.Fatal("expected false for empty product ID lookup on empty mapping")
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for unknown product, got %s", tier)
	}
}

// TestStripeMapping_DuplicateProductIDs verifies that the same product ID used
// for multiple tiers results in last-write-wins behavior.
func TestStripeMapping_DuplicateProductIDs(t *testing.T) {
	t.Parallel()

	// "dup-id" is used for both starter monthly and pro monthly.
	// Since pro monthly is assigned after starter monthly, it wins.
	m := NewStripeMapping("dup-id", "starter-yearly", "dup-id", "pro-yearly")

	tier, ok := m.TierForPrice("dup-id")
	if !ok {
		t.Fatal("expected true for duplicate product ID lookup")
	}
	if tier != domain.PlanPro {
		t.Fatalf("expected PlanPro (last write wins), got %s", tier)
	}
}

// TestTierForPrice_UnknownProduct verifies that an unknown product ID returns PlanFree and false.
func TestTierForPrice_UnknownProduct(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("nonexistent-product-id")
	if ok {
		t.Fatal("expected false for unknown product ID")
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for unknown product, got %s", tier)
	}
}

// TestTierForPrice_EmptyString verifies that an empty string product ID returns PlanFree and false.
func TestTierForPrice_EmptyString(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("")
	if ok {
		t.Fatal("expected false for empty product ID")
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for empty product ID, got %s", tier)
	}
}

// TestTierForPrice_NullBytes verifies that null bytes in a product ID do not cause panics.
func TestTierForPrice_NullBytes(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("starter-m", "starter-y", "pro-m", "pro-y")

	tier, ok := m.TierForPrice("product\x00id")
	if ok {
		t.Fatal("expected false for product ID containing null bytes")
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for null-byte product ID, got %s", tier)
	}
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
			t.Errorf("expected true for product %q", tc.productID)
			continue
		}
		if tier != tc.wantTier {
			t.Errorf("product %q: expected tier %s, got %s", tc.productID, tc.wantTier, tier)
		}
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
		if !ok && tier != domain.PlanFree {
			t.Errorf("unknown product should return PlanFree, got %s", tier)
		}
		if ok && tier != domain.PlanStarter && tier != domain.PlanPro {
			t.Errorf("known product should be Starter or Pro, got %s", tier)
		}
	})
}

// TestStripeMapping_CaseSensitivity verifies that product ID lookups are case-sensitive.
func TestStripeMapping_CaseSensitivity(t *testing.T) {
	t.Parallel()

	m := NewStripeMapping("Starter-M", "starter-y", "pro-m", "pro-y")

	// Exact case should match.
	tier, ok := m.TierForPrice("Starter-M")
	if !ok {
		t.Fatal("expected true for exact-case product ID")
	}
	if tier != domain.PlanStarter {
		t.Fatalf("expected PlanStarter, got %s", tier)
	}

	// Different case should not match.
	_, ok = m.TierForPrice("starter-m")
	if ok {
		t.Fatal("expected false for different-case product ID (case-sensitive)")
	}

	_, ok = m.TierForPrice("STARTER-M")
	if ok {
		t.Fatal("expected false for uppercase product ID (case-sensitive)")
	}
}
