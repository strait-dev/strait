package billing

import "strait/internal/domain"

// PolarMapping maps Polar product IDs to plan tiers and addon types.
type PolarMapping struct {
	productToTier  map[string]domain.PlanTier
	productToAddon map[string]AddonType
}

// PolarMappingOption configures a PolarMapping with product ID to tier mappings.
type PolarMappingOption func(*PolarMapping)

// WithStarterProducts registers Starter plan product IDs.
func WithStarterProducts(monthlyID, yearlyID string) PolarMappingOption {
	return func(m *PolarMapping) {
		if monthlyID != "" {
			m.productToTier[monthlyID] = domain.PlanStarter
		}
		if yearlyID != "" {
			m.productToTier[yearlyID] = domain.PlanStarter
		}
	}
}

// WithProProducts registers Pro plan product IDs.
func WithProProducts(monthlyID, yearlyID string) PolarMappingOption {
	return func(m *PolarMapping) {
		if monthlyID != "" {
			m.productToTier[monthlyID] = domain.PlanPro
		}
		if yearlyID != "" {
			m.productToTier[yearlyID] = domain.PlanPro
		}
	}
}

// WithScaleProducts registers Scale plan product IDs.
func WithScaleProducts(monthlyID, yearlyID string) PolarMappingOption {
	return func(m *PolarMapping) {
		if monthlyID != "" {
			m.productToTier[monthlyID] = domain.PlanScale
		}
		if yearlyID != "" {
			m.productToTier[yearlyID] = domain.PlanScale
		}
	}
}

// WithAddonProduct registers an addon product ID to addon type mapping.
func WithAddonProduct(productID string, addonType AddonType) PolarMappingOption {
	return func(m *PolarMapping) {
		if productID != "" {
			m.productToAddon[productID] = addonType
		}
	}
}

// NewPolarMappingFromOptions creates a mapping from Polar product IDs to plan tiers
// using the provided options.
func NewPolarMappingFromOptions(opts ...PolarMappingOption) *PolarMapping {
	m := &PolarMapping{
		productToTier:  make(map[string]domain.PlanTier),
		productToAddon: make(map[string]AddonType),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// NewPolarMapping creates a mapping from Polar product IDs to plan tiers
// using the provided env var values. This is the legacy constructor that
// supports Starter and Pro. Use NewPolarMappingFromOptions for Scale support.
func NewPolarMapping(
	starterMonthlyID, starterYearlyID string,
	proMonthlyID, proYearlyID string,
) *PolarMapping {
	return NewPolarMappingFromOptions(
		WithStarterProducts(starterMonthlyID, starterYearlyID),
		WithProProducts(proMonthlyID, proYearlyID),
	)
}

// TierForProduct returns the plan tier for a Polar product ID.
// Returns PlanFree and false if the product ID is not mapped.
func (m *PolarMapping) TierForProduct(productID string) (domain.PlanTier, bool) {
	tier, ok := m.productToTier[productID]
	if !ok {
		return domain.PlanFree, false
	}
	return tier, true
}

// AddonTypeForProduct returns the addon type for a Polar product ID.
// Returns empty string and false if the product ID is not an addon.
func (m *PolarMapping) AddonTypeForProduct(productID string) (AddonType, bool) {
	at, ok := m.productToAddon[productID]
	return at, ok
}

// IsAddonProduct returns true if the product ID maps to an addon.
func (m *PolarMapping) IsAddonProduct(productID string) bool {
	_, ok := m.productToAddon[productID]
	return ok
}

// HasProducts returns true if any product IDs are mapped.
func (m *PolarMapping) HasProducts() bool {
	return len(m.productToTier) > 0
}

// ProductCount returns the number of mapped product IDs.
func (m *PolarMapping) ProductCount() int {
	return len(m.productToTier)
}
