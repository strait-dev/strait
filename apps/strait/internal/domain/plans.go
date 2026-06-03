package domain

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
