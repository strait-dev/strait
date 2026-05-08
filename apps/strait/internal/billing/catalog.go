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
}

// PlanCatalogs is the canonical catalog: one entry per Notion tier.
var PlanCatalogs = map[domain.PlanTier]PlanCatalog{
	domain.PlanFree: {
		Tier:                 domain.PlanFree,
		DisplayName:          "Free",
		PriceMonthlyCents:    0,
		PriceAnnualCents:     0,
		LookupKeyMonthly:     "strait_free_monthly",
		LookupKeyOverage:     "strait_overage_free",
		OverageMicrousdPer1K: FreeOveragePerKMicrousd, // $0.50 / 1K (opt-in via CC)
		IncludedRunsPerMonth: MaxRunsPerMonthFree,
		RetentionDays:        RetentionFree,
		Concurrency:          ConcurrentFree,
		Environments:         1,
		LogDrainGB:           0,
	},
	domain.PlanStarter: {
		Tier:                 domain.PlanStarter,
		DisplayName:          "Starter",
		PriceMonthlyCents:    PriceStarterMonthlyCents,
		PriceAnnualCents:     PriceStarterAnnualCents,
		LookupKeyMonthly:     "strait_starter_monthly",
		LookupKeyAnnual:      "strait_starter_annual",
		LookupKeyOverage:     "strait_overage_starter",
		OverageMicrousdPer1K: StarterOveragePerKMicrousd, // $0.40 / 1K
		IncludedRunsPerMonth: MaxRunsPerMonthStarter,
		RetentionDays:        RetentionStarter,
		Concurrency:          ConcurrentStarter,
		Environments:         1,
		LogDrainGB:           5,
	},
	domain.PlanPro: {
		Tier:                 domain.PlanPro,
		DisplayName:          "Pro",
		PriceMonthlyCents:    PriceProMonthlyCents,
		PriceAnnualCents:     PriceProAnnualCents,
		LookupKeyMonthly:     "strait_pro_monthly",
		LookupKeyAnnual:      "strait_pro_annual",
		LookupKeyOverage:     "strait_overage_pro",
		OverageMicrousdPer1K: ProOveragePerKMicrousd, // $0.20 / 1K
		IncludedRunsPerMonth: MaxRunsPerMonthPro,
		RetentionDays:        RetentionPro,
		Concurrency:          ConcurrentPro,
		Environments:         3,
		LogDrainGB:           25,
	},
	domain.PlanScale: {
		Tier:                 domain.PlanScale,
		DisplayName:          "Scale",
		PriceMonthlyCents:    PriceScaleMonthlyCents,
		PriceAnnualCents:     PriceScaleAnnualCents,
		LookupKeyMonthly:     "strait_scale_monthly",
		LookupKeyAnnual:      "strait_scale_annual",
		LookupKeyOverage:     "strait_overage_scale",
		OverageMicrousdPer1K: ScaleOveragePerKMicrousd, // $0.06 / 1K
		IncludedRunsPerMonth: MaxRunsPerMonthScale,
		RetentionDays:        RetentionScale,
		Concurrency:          ConcurrentScale,
		Environments:         10,
		LogDrainGB:           100,
	},
	domain.PlanBusiness: {
		Tier:                 domain.PlanBusiness,
		DisplayName:          "Business",
		PriceMonthlyCents:    PriceBusinessMonthlyCents,
		PriceAnnualCents:     PriceBusinessAnnualCents,
		LookupKeyMonthly:     "strait_business_monthly",
		LookupKeyAnnual:      "strait_business_annual",
		LookupKeyOverage:     "strait_overage_business",
		OverageMicrousdPer1K: BusinessOveragePerKMicrousd, // $0.03 / 1K
		IncludedRunsPerMonth: MaxRunsPerMonthBusiness,
		RetentionDays:        RetentionBusiness,
		Concurrency:          ConcurrentBusiness,
		Environments:         -1,
		LogDrainGB:           500,
	},
	domain.PlanEnterprise: {
		Tier:        domain.PlanEnterprise,
		DisplayName: "Enterprise",
		// Quoted; no Stripe lookup keys.
		IncludedRunsPerMonth: -1,
		RetentionDays:        -1,
		Concurrency:          -1,
		Environments:         -1,
		LogDrainGB:           -1,
		OverageMicrousdPer1K: EnterpriseOveragePerKMicrousd, // $0.03 / 1K (negotiated default)
	},
}

// GetPlanCatalog returns the catalog entry for a tier; falls back to Free.
func GetPlanCatalog(tier domain.PlanTier) PlanCatalog {
	if c, ok := PlanCatalogs[tier]; ok {
		return c
	}
	return PlanCatalogs[domain.PlanFree]
}
