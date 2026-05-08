package billing

import "time"

// AddonType identifies an add-on product.
type AddonType string

const (
	AddonConcurrency100    AddonType = "concurrency_100"
	AddonLogDrain10GB      AddonType = "log_drain_10gb"
	AddonHistory30d        AddonType = "history_30d"
	AddonComplianceArchive AddonType = "compliance_archive"
	AddonDedicatedWorkers  AddonType = "dedicated_workers"
	AddonEnvironments5     AddonType = "environments_5"
)

// AllAddonTypes returns all known add-on types.
func AllAddonTypes() []AddonType {
	return []AddonType{
		AddonConcurrency100,
		AddonLogDrain10GB,
		AddonHistory30d,
		AddonComplianceArchive,
		AddonDedicatedWorkers,
		AddonEnvironments5,
	}
}

// IsValidAddonType returns true if the addon type is recognized.
func IsValidAddonType(t AddonType) bool {
	switch t {
	case AddonConcurrency100, AddonLogDrain10GB, AddonHistory30d,
		AddonComplianceArchive, AddonDedicatedWorkers, AddonEnvironments5:
		return true
	}
	return false
}

// AddonPackDefinition describes the increment and pricing for an add-on pack.
type AddonPackDefinition struct {
	Type        AddonType
	DisplayName string
	LookupKey   string // Stripe lookup_key for cross-account resolution
	PackSize    int    // units per pack (e.g. +50 concurrent runs)
	PriceCents  int    // monthly price in cents
	MaxTotal    int    // maximum total after add-ons; -1 = no cap
}

// AddonPacks defines the available add-on packs.
var AddonPacks = map[AddonType]AddonPackDefinition{
	AddonConcurrency100: {
		Type:        AddonConcurrency100,
		DisplayName: "+100 Concurrent Runs",
		LookupKey:   "strait_addon_concurrency_100",
		PackSize:    100,
		PriceCents:  2000, // $20/mo
		MaxTotal:    -1,
	},
	AddonLogDrain10GB: {
		Type:        AddonLogDrain10GB,
		DisplayName: "+10 GB Log Drain",
		LookupKey:   "strait_addon_log_drain_10gb",
		PackSize:    10,   // +10 GB
		PriceCents:  1500, // $15/mo
		MaxTotal:    -1,
	},
	AddonHistory30d: {
		Type:        AddonHistory30d,
		DisplayName: "+30 Days History",
		LookupKey:   "strait_addon_history_30d",
		PackSize:    30,   // +30 days
		PriceCents:  1000, // $10/mo
		MaxTotal:    -1,
	},
	AddonComplianceArchive: {
		Type:        AddonComplianceArchive,
		DisplayName: "Compliance Archive",
		LookupKey:   "strait_addon_compliance_archive",
		PackSize:    1,
		PriceCents:  10000, // $100/mo
		MaxTotal:    1,
	},
	AddonDedicatedWorkers: {
		Type:        AddonDedicatedWorkers,
		DisplayName: "Dedicated Worker Pool",
		LookupKey:   "strait_addon_dedicated_workers",
		PackSize:    1,
		PriceCents:  20000, // $200/mo
		MaxTotal:    -1,
	},
	AddonEnvironments5: {
		Type:        AddonEnvironments5,
		DisplayName: "+5 Environments",
		LookupKey:   "strait_addon_environments_5",
		PackSize:    5,
		PriceCents:  2500, // $25/mo
		MaxTotal:    -1,
	},
}

// Addon represents an active add-on for an organization.
type Addon struct {
	ID                   string
	OrgID                string
	AddonType            AddonType
	Quantity             int
	StripeSubscriptionID *string
	StripeLookupKey      *string
	Active               bool
	ExpiresAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// EffectiveLimits applies active add-ons to a base plan's limits and returns
// the combined result. Add-ons with invalid types, inactive state, or non-positive
// quantities are silently ignored.
func EffectiveLimits(base OrgPlanLimits, addons []Addon) OrgPlanLimits {
	result := base

	for _, addon := range addons {
		if !addon.Active || addon.Quantity <= 0 {
			continue
		}

		pack, ok := AddonPacks[addon.AddonType]
		if !ok {
			continue
		}

		increment := pack.PackSize * addon.Quantity

		switch addon.AddonType {
		case AddonConcurrency100:
			if result.MaxConcurrentRuns != -1 {
				result.MaxConcurrentRuns += increment
			}
		case AddonLogDrain10GB:
			// LogDrainGB lives in PlanCatalog, not OrgPlanLimits; effect surfaced via catalog merge.
		case AddonHistory30d:
			if result.RetentionDays > 0 {
				result.RetentionDays += increment
			}
		case AddonComplianceArchive:
			result.HasSIEMExport = true
		case AddonDedicatedWorkers:
			result.HasDedicatedCompute = true
		case AddonEnvironments5:
			if result.MaxEnvironments != -1 {
				result.MaxEnvironments += increment
			}
		}
	}

	return result
}
