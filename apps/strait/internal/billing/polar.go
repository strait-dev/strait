package billing

import "strait/internal/domain"

// PolarMapping maps Polar product IDs to plan tiers.
type PolarMapping struct {
	productToTier map[string]domain.PlanTier
}

// NewPolarMapping creates a mapping from Polar product IDs to plan tiers
// using the provided env var values.
func NewPolarMapping(
	starterMonthlyID, starterYearlyID string,
	proMonthlyID, proYearlyID string,
) *PolarMapping {
	m := &PolarMapping{
		productToTier: make(map[string]domain.PlanTier),
	}
	if starterMonthlyID != "" {
		m.productToTier[starterMonthlyID] = domain.PlanStarter
	}
	if starterYearlyID != "" {
		m.productToTier[starterYearlyID] = domain.PlanStarter
	}
	if proMonthlyID != "" {
		m.productToTier[proMonthlyID] = domain.PlanPro
	}
	if proYearlyID != "" {
		m.productToTier[proYearlyID] = domain.PlanPro
	}
	return m
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
