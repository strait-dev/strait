package billing

import "strait/internal/domain"

// PlanCatalog is the Notion-canonical, customer-visible plan shape.
// One entry per tier. Lookup keys are the stable cross-Stripe-account
// identifier used by the billing resolver to fetch live Stripe price IDs.
//
// All monetary values are in cents (USD). All overage rates are in
// micro-USD per 1,000 runs (1 micro-USD = 1e-6 USD).
type PlanCatalog struct {
	Tier                 domain.PlanTier
	DisplayName          string
	PriceMonthlyCents    int
	PriceAnnualCents     int // total annual charge (already 20% off × 12)
	LookupKeyMonthly     string
	LookupKeyAnnual      string // empty for Free and Enterprise
	LookupKeyOverage     string // empty for Enterprise (custom-quoted)
	OverageMicrousdPer1K int64
	IncludedRunsPerMonth int // -1 = unlimited
	RetentionDays        int // -1 = unlimited
	Concurrency          int // -1 = unlimited
	Environments         int // -1 = unlimited
	LogDrainGB           int // -1 = unlimited
	RoadmapFeatures      []string
}

// GetPlanCatalog returns the catalog entry for a tier; falls back to Free.
func GetPlanCatalog(tier domain.PlanTier) PlanCatalog {
	if c, ok := PlanCatalogs[tier]; ok {
		return c
	}
	return PlanCatalogs[domain.PlanFree]
}
