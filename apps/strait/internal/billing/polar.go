package billing

import "strait/internal/domain"

// PolarMapping maps Polar product IDs to plan tiers.
type PolarMapping struct {
	productToTier map[string]domain.PlanTier
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

// NewPolarMapping creates a mapping from Polar product IDs to plan tiers
// using the provided options.
func NewPolarMappingFromOptions(opts ...PolarMappingOption) *PolarMapping {
	m := &PolarMapping{
		productToTier: make(map[string]domain.PlanTier),
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

// HasProducts returns true if any product IDs are mapped.
func (m *PolarMapping) HasProducts() bool {
	return len(m.productToTier) > 0
}

// ProductCount returns the number of mapped product IDs.
func (m *PolarMapping) ProductCount() int {
	return len(m.productToTier)
}
