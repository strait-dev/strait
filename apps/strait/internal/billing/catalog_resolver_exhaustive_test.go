package billing

import (
	"strings"
	"testing"
)

// TestCatalogResolver_TierLookupKeys_RoundTrip walks every entry in
// PlanCatalogs and confirms each declared monthly and annual lookup key
// resolves back to the same tier through the resolver. Catches catalog
// drift: adding a tier without registering its lookup key, or vice versa.
func TestCatalogResolver_TierLookupKeys_RoundTrip(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for tier, cat := range PlanCatalogs {
		if cat.LookupKeyMonthly != "" {
			got, ok := r.TierForLookupKey(cat.LookupKeyMonthly)
			if !ok {
				t.Errorf("tier %q: monthly lookup key %q not registered", tier, cat.LookupKeyMonthly)
			}
			if got != tier {
				t.Errorf("tier %q: monthly lookup key %q resolved to %q", tier, cat.LookupKeyMonthly, got)
			}
		}
		if cat.LookupKeyAnnual != "" {
			got, ok := r.TierForLookupKey(cat.LookupKeyAnnual)
			if !ok {
				t.Errorf("tier %q: annual lookup key %q not registered", tier, cat.LookupKeyAnnual)
			}
			if got != tier {
				t.Errorf("tier %q: annual lookup key %q resolved to %q", tier, cat.LookupKeyAnnual, got)
			}
		}
	}
}

// TestCatalogResolver_AddonLookupKeys_RoundTrip walks every launch-active
// entry in AddonPacks and confirms its lookup key resolves back to the same
// addon type. Roadmap-only add-ons stay in AddonPacks for catalog/docs, but
// must not resolve through the sellable Stripe lookup-key path.
func TestCatalogResolver_AddonLookupKeys_RoundTrip(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for _, pack := range AddonPacks {
		if pack.LookupKey == "" {
			continue
		}
		if !IsLaunchActiveAddonType(pack.Type) {
			if _, ok := r.AddonForLookupKey(pack.LookupKey); ok {
				t.Errorf("roadmap addon %q: lookup key %q must not resolve as sellable addon", pack.Type, pack.LookupKey)
			}
			if r.IsAddonLookupKey(pack.LookupKey) {
				t.Errorf("roadmap addon %q: lookup key %q must not register as addon", pack.Type, pack.LookupKey)
			}
			continue
		}
		got, ok := r.AddonForLookupKey(pack.LookupKey)
		if !ok {
			t.Errorf("addon %q: lookup key %q not registered", pack.Type, pack.LookupKey)
			continue
		}
		if got != pack.Type {
			t.Errorf("addon lookup key %q resolved to %q, want %q", pack.LookupKey, got, pack.Type)
		}
	}
}

// TestCatalogResolver_OverageLookupKeys_NotResolvedAsTier confirms the
// graduated metered overage prices are NOT mistaken for tier prices. Stripe
// delivers subscription events whose first item may be the overage price; the
// resolver must reject it so the handler logs "unknown price" rather than
// silently flipping the org's tier.
func TestCatalogResolver_OverageLookupKeys_NotResolvedAsTier(t *testing.T) {
	t.Parallel()
	r := NewCatalogResolver()

	for tier, cat := range PlanCatalogs {
		if cat.LookupKeyOverage == "" {
			continue
		}
		if _, ok := r.TierForLookupKey(cat.LookupKeyOverage); ok {
			t.Errorf("tier %q: overage lookup key %q must not resolve as a tier", tier, cat.LookupKeyOverage)
		}
		if _, ok := r.AddonForLookupKey(cat.LookupKeyOverage); ok {
			t.Errorf("tier %q: overage lookup key %q must not resolve as an addon", tier, cat.LookupKeyOverage)
		}
		if r.IsAddonLookupKey(cat.LookupKeyOverage) {
			t.Errorf("tier %q: overage lookup key %q must not register as an addon", tier, cat.LookupKeyOverage)
		}
	}
}

// TestCatalogResolver_LookupKeys_StraitPrefix confirms every catalog lookup
// key uses the canonical strait_ prefix. Anything outside this namespace
// would collide with the retired legacy non-canonical naming.
func TestCatalogResolver_LookupKeys_StraitPrefix(t *testing.T) {
	t.Parallel()

	all := allCatalogLookupKeys()
	for _, k := range all {
		if !strings.HasPrefix(k, "strait_") {
			t.Errorf("lookup key %q must use canonical strait_ prefix", k)
		}
		if strings.Contains(k, "_v2") {
			t.Errorf("lookup key %q still carries the temporary _v2 suffix", k)
		}
		if k != strings.ToLower(k) {
			t.Errorf("lookup key %q must be lowercase", k)
		}
	}
}

// TestCatalogResolver_LookupKeys_Unique confirms no two catalog entries
// share a lookup key. A collision would silently steer Stripe events into
// the wrong handler.
func TestCatalogResolver_LookupKeys_Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string)
	check := func(key, owner string) {
		if key == "" {
			return
		}
		if prev, exists := seen[key]; exists {
			t.Errorf("lookup key %q is shared by %q and %q", key, prev, owner)
			return
		}
		seen[key] = owner
	}

	for tier, cat := range PlanCatalogs {
		check(cat.LookupKeyMonthly, "tier:"+string(tier)+":monthly")
		check(cat.LookupKeyAnnual, "tier:"+string(tier)+":annual")
		check(cat.LookupKeyOverage, "tier:"+string(tier)+":overage")
	}
	for _, pack := range AddonPacks {
		check(pack.LookupKey, "addon:"+string(pack.Type))
	}
}

func allCatalogLookupKeys() []string {
	var keys []string
	for _, cat := range PlanCatalogs {
		if cat.LookupKeyMonthly != "" {
			keys = append(keys, cat.LookupKeyMonthly)
		}
		if cat.LookupKeyAnnual != "" {
			keys = append(keys, cat.LookupKeyAnnual)
		}
		if cat.LookupKeyOverage != "" {
			keys = append(keys, cat.LookupKeyOverage)
		}
	}
	for _, pack := range AddonPacks {
		if pack.LookupKey != "" {
			keys = append(keys, pack.LookupKey)
		}
	}
	return keys
}
