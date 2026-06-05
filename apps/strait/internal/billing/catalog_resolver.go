package billing

import "strait/internal/domain"

// CatalogResolver maps Stripe lookup keys (cross-account stable identifiers)
// to plan tiers and addon types based on the canonical catalog. Lookup keys
// replace per-account Stripe Price IDs as the resolution mechanism so the
// same code can run against any Stripe account (sandbox, live, future
// accounts) without re-binding price IDs.
type CatalogResolver struct {
	tierByLookupKey  map[string]domain.PlanTier
	addonByLookupKey map[string]AddonType
}

// NewCatalogResolver builds a resolver from the canonical PlanCatalogs and
// launch-active AddonPacks. Empty lookup keys (e.g. Free annual, Enterprise)
// and roadmap-only add-ons are skipped.
func NewCatalogResolver() *CatalogResolver {
	r := &CatalogResolver{
		tierByLookupKey:  make(map[string]domain.PlanTier),
		addonByLookupKey: make(map[string]AddonType),
	}
	for tier, c := range PlanCatalogs {
		if c.LookupKeyMonthly != "" {
			r.tierByLookupKey[c.LookupKeyMonthly] = tier
		}
		if c.LookupKeyAnnual != "" {
			r.tierByLookupKey[c.LookupKeyAnnual] = tier
		}
	}
	for _, pack := range AddonPacks {
		if !IsLaunchActiveAddonType(pack.Type) {
			continue
		}
		if pack.LookupKey != "" {
			r.addonByLookupKey[pack.LookupKey] = pack.Type
		}
	}
	return r
}

// TierForLookupKey returns the plan tier for a Stripe lookup key.
// Returns PlanFree and false when unmapped.
func (r *CatalogResolver) TierForLookupKey(lookupKey string) (domain.PlanTier, bool) {
	if r == nil || lookupKey == "" {
		return domain.PlanFree, false
	}
	t, ok := r.tierByLookupKey[lookupKey]
	if !ok {
		return domain.PlanFree, false
	}
	return t, true
}

// AddonForLookupKey returns the addon type for a Stripe lookup key.
// Returns empty AddonType and false when unmapped.
func (r *CatalogResolver) AddonForLookupKey(lookupKey string) (AddonType, bool) {
	if r == nil || lookupKey == "" {
		return "", false
	}
	a, ok := r.addonByLookupKey[lookupKey]
	return a, ok
}

// IsAddonLookupKey reports whether the lookup key resolves to an addon.
func (r *CatalogResolver) IsAddonLookupKey(lookupKey string) bool {
	if r == nil || lookupKey == "" {
		return false
	}
	_, ok := r.addonByLookupKey[lookupKey]
	return ok
}

// TierCount returns the number of registered tier lookup keys.
func (r *CatalogResolver) TierCount() int {
	if r == nil {
		return 0
	}
	return len(r.tierByLookupKey)
}

// AddonCount returns the number of registered addon lookup keys.
func (r *CatalogResolver) AddonCount() int {
	if r == nil {
		return 0
	}
	return len(r.addonByLookupKey)
}
