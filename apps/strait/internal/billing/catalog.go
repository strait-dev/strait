package billing

import "strait/internal/domain"

// PlanCatalog is the Notion-canonical, customer-visible plan shape.
// One entry per tier. Lookup keys are the stable cross-Stripe-account
// identifier used by the billing resolver to fetch live Stripe price IDs.
//
// All monetary values are in cents (USD). All overage rates are in
// micro-USD per 1,000 runs (1 micro-USD = 1e-6 USD).
type PlanCatalog struct {
	Tier                       domain.PlanTier
	DisplayName                string
	PriceMonthlyCents          int
	PriceAnnualCents           int // total annual charge (already 20% off × 12)
	LookupKeyMonthly           string
	LookupKeyAnnual            string // empty for Free and Enterprise
	LookupKeyOverage           string // empty for Enterprise (custom-quoted)
	OverageMicrousdPer1K       int64
	OverageDefaultEnabled      bool
	DefaultSpendingCapMicrousd int64
	IncludedRunsPerMonth       int // -1 = unlimited
	RetentionDays              int // -1 = unlimited
	Concurrency                int // -1 = unlimited
	Environments               int // -1 = unlimited
	LogDrainGB                 int // -1 = unlimited
	RoadmapFeatures            []string
}

// AddonCatalog is the generated, customer-visible add-on product shape.
// Launch-active add-ons have a Stripe lookup key; roadmap add-ons keep an empty
// lookup key so checkout code cannot bind them accidentally.
type AddonCatalog struct {
	Type        AddonType
	DisplayName string
	LookupKey   string
	PackSize    int
	PriceCents  int
	MaxTotal    int
	Status      string
	AvailableOn []domain.PlanTier
}

// GetPlanCatalog returns the catalog entry for a tier; falls back to Free.
func GetPlanCatalog(tier domain.PlanTier) PlanCatalog {
	if c, ok := PlanCatalogs[tier]; ok {
		return c
	}
	return PlanCatalogs[domain.PlanFree]
}

// GetAddonCatalog returns the catalog entry for an add-on type.
func GetAddonCatalog(t AddonType) (AddonCatalog, bool) {
	c, ok := AddonCatalogs[t]
	return c, ok
}
