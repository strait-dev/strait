package billing

import (
	"testing"

	"strait/internal/domain"
)

func TestCatalogResolver_TierForLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	cases := []struct {
		key  string
		want domain.PlanTier
	}{
		{"strait_free_monthly", domain.PlanFree},
		{"strait_starter_monthly", domain.PlanStarter},
		{"strait_starter_annual", domain.PlanStarter},
		{"strait_pro_monthly", domain.PlanPro},
		{"strait_pro_annual", domain.PlanPro},
		{"strait_scale_monthly", domain.PlanScale},
		{"strait_scale_annual", domain.PlanScale},
		{"strait_business_monthly", domain.PlanBusiness},
		{"strait_business_annual", domain.PlanBusiness},
	}

	for _, c := range cases {
		got, ok := r.TierForLookupKey(c.key)
		if !ok || got != c.want {
			t.Errorf("TierForLookupKey(%q) = (%q, %v), want (%q, true)", c.key, got, ok, c.want)
		}
	}
}

func TestCatalogResolver_TierForLookupKey_Unmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for _, key := range []string{"", "unknown_key", "strait_free_annual" /* free has no annual */} {
		if _, ok := r.TierForLookupKey(key); ok {
			t.Errorf("TierForLookupKey(%q) should be unmapped", key)
		}
	}
}

func TestCatalogResolver_TierForLookupKey_EnterpriseUnmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	if _, ok := r.TierForLookupKey("strait_enterprise_monthly"); ok {
		t.Error("Enterprise has no lookup keys (custom-quoted); should be unmapped")
	}
}

func TestCatalogResolver_AddonForLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	cases := []struct {
		key  string
		want AddonType
	}{
		{"strait_addon_concurrency_100", AddonConcurrency100},
		{"strait_addon_history_30d", AddonHistory30d},
		{"strait_addon_environments_5", AddonEnvironments5},
	}

	for _, c := range cases {
		got, ok := r.AddonForLookupKey(c.key)
		if !ok || got != c.want {
			t.Errorf("AddonForLookupKey(%q) = (%q, %v), want (%q, true)", c.key, got, ok, c.want)
		}
	}
}

func TestCatalogResolver_RoadmapAddonLookupKeysUnmapped(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for _, key := range []string{
		"strait_addon_compliance_archive",
		"strait_addon_dedicated_pool",
	} {
		if _, ok := r.AddonForLookupKey(key); ok {
			t.Errorf("roadmap addon lookup key %q must not resolve as sellable addon", key)
		}
		if r.IsAddonLookupKey(key) {
			t.Errorf("roadmap addon lookup key %q must not register as addon", key)
		}
	}
}

func TestCatalogResolver_IsAddonLookupKey(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	if !r.IsAddonLookupKey("strait_addon_concurrency_100") {
		t.Error("expected concurrency addon lookup key to be recognized")
	}
	if r.IsAddonLookupKey("strait_pro_monthly") {
		t.Error("plan tier lookup key must not register as addon")
	}
	if r.IsAddonLookupKey("strait_addon_log_drain_10gb") {
		t.Error("removed log-drain volume add-on must not register as addon")
	}
	if r.IsAddonLookupKey("") {
		t.Error("empty lookup key must not register as addon")
	}
}

func TestCatalogResolver_NilSafe(t *testing.T) {
	t.Parallel()
	var r *CatalogResolver

	if _, ok := r.TierForLookupKey("anything"); ok {
		t.Error("nil resolver must not resolve a tier")
	}
	if _, ok := r.AddonForLookupKey("anything"); ok {
		t.Error("nil resolver must not resolve an addon")
	}
	if r.IsAddonLookupKey("anything") {
		t.Error("nil resolver must not report addon lookup keys")
	}
	if r.TierCount() != 0 || r.AddonCount() != 0 {
		t.Error("nil resolver counts must be zero")
	}
}

func TestCatalogResolver_Counts(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	// 4 tiers (Starter, Pro, Scale, Business) have monthly + annual = 8.
	// Free has only monthly = 1. Enterprise has none. Total = 9.
	if got := r.TierCount(); got != 9 {
		t.Errorf("TierCount() = %d, want 9", got)
	}

	// Only launch-active addons resolve as sellable Stripe addon lookup keys.
	if got := r.AddonCount(); got != 3 {
		t.Errorf("AddonCount() = %d, want 3", got)
	}
}

func TestCatalogResolver_BusinessTier(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	got, ok := r.TierForLookupKey("strait_business_monthly")
	if !ok {
		t.Fatal("business monthly lookup key not registered")
	}
	if got != domain.PlanBusiness {
		t.Errorf("strait_business_monthly resolved to %q, want %q", got, domain.PlanBusiness)
	}

	gotAnnual, ok := r.TierForLookupKey("strait_business_annual")
	if !ok {
		t.Fatal("business annual lookup key not registered")
	}
	if gotAnnual != domain.PlanBusiness {
		t.Errorf("strait_business_annual resolved to %q, want %q", gotAnnual, domain.PlanBusiness)
	}
}
