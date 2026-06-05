package billing

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.True(t, ok)
			assert.Equal(t, tier,
				got)

		}
		if cat.LookupKeyAnnual != "" {
			got, ok := r.TierForLookupKey(cat.LookupKeyAnnual)
			assert.True(t, ok)
			assert.Equal(t, tier,
				got)

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
				assert.Failf(t, "test failure",

					"roadmap addon %q: lookup key %q must not resolve as sellable addon", pack.Type, pack.LookupKey)
			}
			assert.False(t, r.
				IsAddonLookupKey(pack.LookupKey))

			continue
		}
		got, ok := r.AddonForLookupKey(pack.LookupKey)
		if !ok {
			assert.Failf(t, "test failure",

				"addon %q: lookup key %q not registered", pack.Type, pack.LookupKey)
			continue
		}
		assert.Equal(t, pack.
			Type, got,
		)

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
			assert.Failf(t, "test failure",

				"tier %q: overage lookup key %q must not resolve as a tier", tier, cat.LookupKeyOverage)
		}
		if _, ok := r.AddonForLookupKey(cat.LookupKeyOverage); ok {
			assert.Failf(t, "test failure",

				"tier %q: overage lookup key %q must not resolve as an addon", tier, cat.LookupKeyOverage)
		}
		assert.False(t, r.
			IsAddonLookupKey(cat.LookupKeyOverage))

	}
}

// TestCatalogResolver_LookupKeys_StraitPrefix confirms every catalog lookup
// key uses the canonical strait_ prefix. Anything outside this namespace
// would collide with the retired legacy non-canonical naming.
func TestCatalogResolver_LookupKeys_StraitPrefix(t *testing.T) {
	t.Parallel()

	all := allCatalogLookupKeys()
	for _, k := range all {
		assert.True(t, strings.HasPrefix(k, "strait_"))
		assert.False(t, strings.Contains(k, "_v2"))
		assert.Equal(t, strings.ToLower(k), k)

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
			assert.Failf(t, "test failure",

				"lookup key %q is shared by %q and %q", key, prev, owner)
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
