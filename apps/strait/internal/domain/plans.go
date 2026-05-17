package domain

import "slices"

// PlanTier represents a pricing tier that determines region access.
type PlanTier string

const (
	PlanFree       PlanTier = "free"
	PlanStarter    PlanTier = "starter"
	PlanPro        PlanTier = "pro"
	PlanScale      PlanTier = "scale"
	PlanBusiness   PlanTier = "business"
	PlanEnterprise PlanTier = "enterprise"
)

// AllPlanTiers returns all valid plan tiers in ascending order.
func AllPlanTiers() []PlanTier {
	return []PlanTier{PlanFree, PlanStarter, PlanPro, PlanScale, PlanBusiness, PlanEnterprise}
}

// IsValid returns true if the plan tier is a recognized value.
func (p PlanTier) IsValid() bool {
	switch p {
	case PlanFree, PlanStarter, PlanPro, PlanScale, PlanBusiness, PlanEnterprise:
		return true
	}
	return false
}

// Rank returns the numeric rank of a plan tier (0=free, 5=enterprise).
// Unknown tiers return 0.
func (p PlanTier) Rank() int {
	switch p {
	case PlanFree:
		return 0
	case PlanStarter:
		return 1
	case PlanPro:
		return 2
	case PlanScale:
		return 3
	case PlanBusiness:
		return 4
	case PlanEnterprise:
		return 5
	default:
		return 0
	}
}

// PlanConfig defines the capabilities of a plan tier.
type PlanConfig struct {
	Tier           PlanTier
	MaxRegions     int      // Max regions for multi-region preference list
	AllowedRegions []string // Empty means all regions
	MultiRegion    bool     // Can configure preferred_regions list
}

// AllPlanConfigs returns the configuration for all plan tiers.
func AllPlanConfigs() map[PlanTier]PlanConfig {
	return map[PlanTier]PlanConfig{
		PlanFree: {
			Tier:           PlanFree,
			MaxRegions:     1,
			AllowedRegions: []string{"iad"},
			MultiRegion:    false,
		},
		PlanStarter: {
			Tier:           PlanStarter,
			MaxRegions:     1,
			AllowedRegions: []string{"iad", "ord", "lax", "lhr", "fra", "sin"},
			MultiRegion:    false,
		},
		PlanPro: {
			Tier:           PlanPro,
			MaxRegions:     3,
			AllowedRegions: nil, // all regions
			MultiRegion:    true,
		},
		PlanScale: {
			Tier:           PlanScale,
			MaxRegions:     5,
			AllowedRegions: nil, // all regions
			MultiRegion:    true,
		},
		PlanBusiness: {
			Tier:           PlanBusiness,
			MaxRegions:     5,
			AllowedRegions: nil, // all regions
			MultiRegion:    true,
		},
		PlanEnterprise: {
			Tier:           PlanEnterprise,
			MaxRegions:     5,
			AllowedRegions: nil, // all regions
			MultiRegion:    true,
		},
	}
}

// GetPlanConfig returns the plan configuration for the given tier.
// Returns the free plan config if the tier is unknown.
func GetPlanConfig(tier PlanTier) PlanConfig {
	configs := AllPlanConfigs()
	if cfg, ok := configs[tier]; ok {
		return cfg
	}
	return configs[PlanFree]
}

// IsRegionAllowed checks if a region is allowed for the given plan tier.
func IsRegionAllowed(tier PlanTier, region string) bool {
	cfg := GetPlanConfig(tier)
	if len(cfg.AllowedRegions) == 0 {
		return true // no restriction
	}
	return slices.Contains(cfg.AllowedRegions, region)
}
