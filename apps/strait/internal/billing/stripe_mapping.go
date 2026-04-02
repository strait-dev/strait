package billing

import "strait/internal/domain"

// StripeMapping maps Stripe Price IDs to plan tiers and addon types.
type StripeMapping struct {
	priceToTier  map[string]domain.PlanTier
	priceToAddon map[string]AddonType
}

// StripeMappingOption configures a StripeMapping with Price ID to tier mappings.
type StripeMappingOption func(*StripeMapping)

// WithStarterPrices registers Starter plan Price IDs.
func WithStarterPrices(monthlyID, yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if monthlyID != "" {
			m.priceToTier[monthlyID] = domain.PlanStarter
		}
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanStarter
		}
	}
}

// WithProPrices registers Pro plan Price IDs.
func WithProPrices(monthlyID, yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if monthlyID != "" {
			m.priceToTier[monthlyID] = domain.PlanPro
		}
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanPro
		}
	}
}

// WithScalePrices registers Scale plan Price IDs.
func WithScalePrices(monthlyID, yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if monthlyID != "" {
			m.priceToTier[monthlyID] = domain.PlanScale
		}
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanScale
		}
	}
}

// WithEnterpriseStarterPrice registers the Enterprise Starter plan yearly Price ID.
func WithEnterpriseStarterPrice(yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanEnterprise
			RegisterEnterprisePriceTier(yearlyID, EnterpriseTierStarter)
		}
	}
}

// WithEnterpriseGrowthPrice registers the Enterprise Growth plan yearly Price ID.
func WithEnterpriseGrowthPrice(yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanEnterprise
			RegisterEnterprisePriceTier(yearlyID, EnterpriseTierGrowth)
		}
	}
}

// WithEnterpriseLargePrice registers the Enterprise Large plan yearly Price ID.
func WithEnterpriseLargePrice(yearlyID string) StripeMappingOption {
	return func(m *StripeMapping) {
		if yearlyID != "" {
			m.priceToTier[yearlyID] = domain.PlanEnterprise
			RegisterEnterprisePriceTier(yearlyID, EnterpriseTierLarge)
		}
	}
}

// WithAddonPrice registers an addon Price ID to addon type mapping.
func WithAddonPrice(priceID string, addonType AddonType) StripeMappingOption {
	return func(m *StripeMapping) {
		if priceID != "" {
			m.priceToAddon[priceID] = addonType
		}
	}
}

// NewStripeMappingFromOptions creates a mapping from Stripe Price IDs to plan tiers
// using the provided options.
func NewStripeMappingFromOptions(opts ...StripeMappingOption) *StripeMapping {
	m := &StripeMapping{
		priceToTier:  make(map[string]domain.PlanTier),
		priceToAddon: make(map[string]AddonType),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// NewStripeMapping creates a mapping from Stripe Price IDs to plan tiers
// using the provided env var values. Supports Starter and Pro.
// Use NewStripeMappingFromOptions for Scale support.
func NewStripeMapping(
	starterMonthlyID, starterYearlyID string,
	proMonthlyID, proYearlyID string,
) *StripeMapping {
	return NewStripeMappingFromOptions(
		WithStarterPrices(starterMonthlyID, starterYearlyID),
		WithProPrices(proMonthlyID, proYearlyID),
	)
}

// TierForPrice returns the plan tier for a Stripe Price ID.
// Returns PlanFree and false if the Price ID is not mapped.
func (m *StripeMapping) TierForPrice(priceID string) (domain.PlanTier, bool) {
	tier, ok := m.priceToTier[priceID]
	if !ok {
		return domain.PlanFree, false
	}
	return tier, true
}

// AddonTypeForPrice returns the addon type for a Stripe Price ID.
// Returns empty string and false if the Price ID is not an addon.
func (m *StripeMapping) AddonTypeForPrice(priceID string) (AddonType, bool) {
	at, ok := m.priceToAddon[priceID]
	return at, ok
}

// IsAddonPrice returns true if the Price ID maps to an addon.
func (m *StripeMapping) IsAddonPrice(priceID string) bool {
	_, ok := m.priceToAddon[priceID]
	return ok
}

// HasPrices returns true if any Price IDs are mapped.
func (m *StripeMapping) HasPrices() bool {
	return len(m.priceToTier) > 0
}

// PriceCount returns the number of mapped Price IDs.
func (m *StripeMapping) PriceCount() int {
	return len(m.priceToTier)
}
