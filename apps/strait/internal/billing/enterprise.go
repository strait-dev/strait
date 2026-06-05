package billing

import (
	"errors"
	"math"
	"sync"
	"time"
)

// EnterpriseTier identifies the Enterprise sub-tier (commercial terms, not features).
// All enterprise sub-tiers share the same plan-level feature set (domain.PlanEnterprise);
// only the commercial terms differ (commitment, credits, discount, SLA).
type EnterpriseTier string

const (
	EnterpriseTierStarter EnterpriseTier = "enterprise_starter"
	EnterpriseTierGrowth  EnterpriseTier = "enterprise_growth"
	EnterpriseTierLarge   EnterpriseTier = "enterprise_large"
)

// AllEnterpriseTiers returns all valid enterprise sub-tiers.
func AllEnterpriseTiers() []EnterpriseTier {
	return []EnterpriseTier{
		EnterpriseTierStarter,
		EnterpriseTierGrowth,
		EnterpriseTierLarge,
	}
}

// IsValidEnterpriseTier returns true if the tier string is a recognized enterprise sub-tier.
func IsValidEnterpriseTier(tier EnterpriseTier) bool {
	switch tier {
	case EnterpriseTierStarter, EnterpriseTierGrowth, EnterpriseTierLarge:
		return true
	}
	return false
}

// EnterpriseConfig holds the commercial terms for an enterprise sub-tier.
// These are default sales templates; individual contracts may override
// OverageDiscountPct.
type EnterpriseConfig struct {
	Tier                   EnterpriseTier
	DisplayName            string
	AnnualCommitmentCents  int64   // $18,000 = 1_800_000
	MonthlyEquivalentCents int64   // $1,500 = 150_000
	PlatformFeeMicrousd    int64   // monthly platform fee in micro-USD
	OverageDiscountPct     int     // 10, 15, or 20
	UptimeSLAPct           float64 // 99.9 or 99.95
	MaxDowntimeMinutes     float64 // per month
	SupportResponseP1      string  // "1h"
	SupportResponseP2      string  // "4h"
	SupportResponseP3      string  // "24h"
}

// Enterprise pricing constants.
const (
	// Annual commitments in cents.
	EnterpriseStarterAnnualCents int64 = 1_800_000 // $18,000
	EnterpriseGrowthAnnualCents  int64 = 4_800_000 // $48,000
	EnterpriseLargeAnnualCents   int64 = 9_600_000 // $96,000

	// Monthly equivalents in cents.
	EnterpriseStarterMonthlyCents int64 = 150_000 // $1,500
	EnterpriseGrowthMonthlyCents  int64 = 400_000 // $4,000
	EnterpriseLargeMonthlyCents   int64 = 800_000 // $8,000

	// Monthly platform fees in micro-USD.
	EnterpriseStarterPlatformFeeMicrousd int64 = 1_500_000_000 // $1,500
	EnterpriseGrowthPlatformFeeMicrousd  int64 = 4_000_000_000 // $4,000

	// Overage discounts (percentage off standard orchestration overage rates).
	EnterpriseStarterOverageDiscountPct = 10
	EnterpriseGrowthOverageDiscountPct  = 15
	EnterpriseLargeOverageDiscountPct   = 20

	// SLA uptime percentages.
	EnterpriseStarterSLAPct = 99.9
	EnterpriseGrowthSLAPct  = 99.95
	EnterpriseLargeSLAPct   = 99.95

	// Max downtime minutes per month.
	EnterpriseStarterMaxDowntime = 43.8
	EnterpriseGrowthMaxDowntime  = 21.9
	EnterpriseLargeMaxDowntime   = 21.9
)

// EnterpriseConfigs maps enterprise sub-tiers to their default commercial terms.
var EnterpriseConfigs = map[EnterpriseTier]EnterpriseConfig{
	EnterpriseTierStarter: {
		Tier:                   EnterpriseTierStarter,
		DisplayName:            "Starter Enterprise",
		AnnualCommitmentCents:  EnterpriseStarterAnnualCents,
		MonthlyEquivalentCents: EnterpriseStarterMonthlyCents,
		PlatformFeeMicrousd:    EnterpriseStarterPlatformFeeMicrousd,
		OverageDiscountPct:     EnterpriseStarterOverageDiscountPct,
		UptimeSLAPct:           EnterpriseStarterSLAPct,
		MaxDowntimeMinutes:     EnterpriseStarterMaxDowntime,
		SupportResponseP1:      "1h",
		SupportResponseP2:      "4h",
		SupportResponseP3:      "24h",
	},
	EnterpriseTierGrowth: {
		Tier:                   EnterpriseTierGrowth,
		DisplayName:            "Growth Enterprise",
		AnnualCommitmentCents:  EnterpriseGrowthAnnualCents,
		MonthlyEquivalentCents: EnterpriseGrowthMonthlyCents,
		PlatformFeeMicrousd:    EnterpriseGrowthPlatformFeeMicrousd,
		OverageDiscountPct:     EnterpriseGrowthOverageDiscountPct,
		UptimeSLAPct:           EnterpriseGrowthSLAPct,
		MaxDowntimeMinutes:     EnterpriseGrowthMaxDowntime,
		SupportResponseP1:      "1h",
		SupportResponseP2:      "4h",
		SupportResponseP3:      "24h",
	},
	EnterpriseTierLarge: {
		Tier:                   EnterpriseTierLarge,
		DisplayName:            "Large Enterprise",
		AnnualCommitmentCents:  EnterpriseLargeAnnualCents,
		MonthlyEquivalentCents: EnterpriseLargeMonthlyCents,
		PlatformFeeMicrousd:    0, // custom/negotiated
		OverageDiscountPct:     EnterpriseLargeOverageDiscountPct,
		UptimeSLAPct:           EnterpriseLargeSLAPct,
		MaxDowntimeMinutes:     EnterpriseLargeMaxDowntime,
		SupportResponseP1:      "1h",
		SupportResponseP2:      "4h",
		SupportResponseP3:      "24h",
	},
}

// GetEnterpriseConfig returns the default config for an enterprise sub-tier.
// Returns the starter config if the tier is unknown.
func GetEnterpriseConfig(tier EnterpriseTier) EnterpriseConfig {
	if cfg, ok := EnterpriseConfigs[tier]; ok {
		return cfg
	}
	return EnterpriseConfigs[EnterpriseTierStarter]
}

// enterprisePriceToTier maps Stripe price IDs to enterprise sub-tiers.
// Populated by WithEnterprise*Price options on StripeMapping.
var (
	enterprisePriceToTier   = make(map[string]EnterpriseTier)
	enterprisePriceToTierMu sync.RWMutex
)

// EnterpriseTierForPrice returns the enterprise sub-tier for a Stripe price ID.
// Returns empty string and false if the price is not an enterprise price.
func EnterpriseTierForPrice(priceID string) (EnterpriseTier, bool) {
	enterprisePriceToTierMu.RLock()
	defer enterprisePriceToTierMu.RUnlock()
	tier, ok := enterprisePriceToTier[priceID]
	return tier, ok
}

// RegisterEnterprisePriceTier associates a Stripe price ID with an enterprise sub-tier.
// Called by the WithEnterprise*Price stripe mapping options.
func RegisterEnterprisePriceTier(priceID string, tier EnterpriseTier) {
	if priceID != "" {
		enterprisePriceToTierMu.Lock()
		enterprisePriceToTier[priceID] = tier
		enterprisePriceToTierMu.Unlock()
	}
}

// ApplyOverageDiscount reduces a cost by the given discount percentage.
// Returns the discounted cost in micro-USD. Negative costs return 0.
func ApplyOverageDiscount(costMicro int64, discountPct int) int64 {
	if costMicro <= 0 {
		return 0
	}
	if discountPct <= 0 {
		return costMicro
	}
	if discountPct >= 100 {
		return 0
	}
	return costMicro * int64(100-discountPct) / 100
}

// EnterpriseContract represents an organization's enterprise contract terms.
type EnterpriseContract struct {
	ID                    string
	OrgID                 string
	EnterpriseTier        EnterpriseTier
	AnnualCommitmentCents int64
	OverageDiscountPct    int
	ContractStartDate     time.Time
	ContractEndDate       time.Time
	AutoRenew             bool
	BillingCadence        string // "annual", "quarterly"
	StripeSubscriptionID  *string
	Notes                 string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Sentinel errors for enterprise contract operations.
var (
	ErrContractNotFound = errors.New("enterprise contract not found")
)

// ValidBillingCadences are the allowed billing cadence values.
var ValidBillingCadences = []string{"annual", "quarterly"}

// IsValidBillingCadence returns true if the cadence is recognized.
func IsValidBillingCadence(cadence string) bool {
	switch cadence {
	case "annual", "quarterly":
		return true
	}
	return false
}

// ValidateEnterpriseContract checks that a contract's fields are consistent.
func ValidateEnterpriseContract(c *EnterpriseContract) error {
	if c.OrgID == "" {
		return errors.New("org_id is required")
	}
	if !IsValidEnterpriseTier(c.EnterpriseTier) {
		return errors.New("invalid enterprise tier")
	}
	if c.AnnualCommitmentCents < EnterpriseStarterAnnualCents {
		return errors.New("annual commitment below minimum ($18,000)")
	}
	if c.OverageDiscountPct < 0 || c.OverageDiscountPct > 100 {
		return errors.New("overage discount must be between 0 and 100")
	}
	if !c.ContractEndDate.After(c.ContractStartDate) {
		return errors.New("contract end date must be after start date")
	}
	if !IsValidBillingCadence(c.BillingCadence) {
		return errors.New("invalid billing cadence")
	}
	return nil
}

// SLA credit remedies per the enterprise pricing doc.
// Applied automatically when monthly uptime falls below SLA threshold.

// SLACreditTier defines a credit remedy for an uptime range.
type SLACreditTier struct {
	MinUptimePct float64 // inclusive lower bound
	MaxUptimePct float64 // exclusive upper bound
	CreditPct    int     // percentage of monthly base fee credited
}

// SLACreditTiers defines the credit remedies from the pricing doc.
var SLACreditTiers = []SLACreditTier{
	{MinUptimePct: 99.0, MaxUptimePct: 99.9, CreditPct: 10},
	{MinUptimePct: 95.0, MaxUptimePct: 99.0, CreditPct: 25},
	{MinUptimePct: 0.0, MaxUptimePct: 95.0, CreditPct: 50},
}

// CalculateSLACredit returns the credit percentage for a given monthly uptime.
// The slaTarget parameter specifies the per-tier SLA threshold (e.g. 99.9 for
// Starter, 99.95 for Growth/Large). Returns 0 if uptime is at or above the target.
// For tiers with a higher SLA target, uptimes between the highest credit tier
// boundary and the target receive the lightest credit (10%).
func CalculateSLACredit(uptimePct float64, slaTarget float64) int {
	if math.IsNaN(uptimePct) || math.IsNaN(slaTarget) || slaTarget <= 0 {
		return 0
	}
	if uptimePct >= slaTarget {
		return 0
	}
	for _, tier := range SLACreditTiers {
		// Extend the top credit tier's upper bound to cover the SLA target.
		maxPct := tier.MaxUptimePct
		if slaTarget > maxPct && tier.CreditPct == SLACreditTiers[0].CreditPct {
			maxPct = slaTarget
		}
		if uptimePct >= tier.MinUptimePct && uptimePct < maxPct {
			return tier.CreditPct
		}
	}
	return 50 // below 95%
}
