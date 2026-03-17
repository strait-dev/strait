package domain

// PlanTier represents a pricing tier that determines region access.
type PlanTier string

const (
	PlanFree         PlanTier = "free"
	PlanStarter      PlanTier = "starter"
	PlanProfessional PlanTier = "professional"
	PlanEnterprise   PlanTier = "enterprise"
)

// IsValid returns true if the plan tier is a recognized value.
func (p PlanTier) IsValid() bool {
	switch p {
	case PlanFree, PlanStarter, PlanProfessional, PlanEnterprise:
		return true
	}
	return false
}

// PlanConfig defines the capabilities of a plan tier.
type PlanConfig struct {
	Tier              PlanTier
	MaxRegions        int      // Max regions for multi-region preference list
	AllowedRegions    []string // Empty means all regions
	MultiRegion       bool     // Can configure preferred_regions list
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
			AllowedRegions: []string{"iad", "lax", "lhr", "fra", "nrt", "syd"},
			MultiRegion:    false,
		},
		PlanProfessional: {
			Tier:           PlanProfessional,
			MaxRegions:     3,
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
	for _, r := range cfg.AllowedRegions {
		if r == region {
			return true
		}
	}
	return false
}
