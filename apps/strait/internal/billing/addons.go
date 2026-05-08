package billing

import "time"

// AddonType identifies an add-on product.
type AddonType string

const (
	// New canonical addons (Notion catalog).
	AddonConcurrency100    AddonType = "concurrency_100"
	AddonLogDrain10GB      AddonType = "log_drain_10gb"
	AddonHistory30d        AddonType = "history_30d"
	AddonComplianceArchive AddonType = "compliance_archive"
	AddonDedicatedWorkers  AddonType = "dedicated_workers"
	AddonEnvironments5     AddonType = "environments_5"

	// Deprecated: replaced by the canonical addons above.
	// Kept for compile compatibility while Phase 2b rewrites callers/tests.
	AddonConcurrentRuns   AddonType = "concurrent_runs"
	AddonMembers          AddonType = "members"
	AddonCronSchedules    AddonType = "cron_schedules"
	AddonDataRetention    AddonType = "data_retention"
	AddonWebhookEndpoints AddonType = "webhook_endpoints"
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
		AddonConcurrentRuns,
		AddonMembers,
		AddonCronSchedules,
		AddonDataRetention,
		AddonWebhookEndpoints,
	}
}

// IsValidAddonType returns true if the addon type is recognized.
func IsValidAddonType(t AddonType) bool {
	switch t {
	case AddonConcurrency100, AddonLogDrain10GB, AddonHistory30d,
		AddonComplianceArchive, AddonDedicatedWorkers, AddonEnvironments5,
		AddonConcurrentRuns, AddonMembers, AddonCronSchedules,
		AddonDataRetention, AddonWebhookEndpoints:
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

	// Deprecated entries — kept for legacy callers/tests; removed in Phase 2b.
	AddonConcurrentRuns: {
		Type:        AddonConcurrentRuns,
		DisplayName: "Concurrent Runs",
		PackSize:    50,
		PriceCents:  1000, // $10/mo
		MaxTotal:    -1,
	},
	AddonMembers: {
		Type:        AddonMembers,
		DisplayName: "Team Members",
		PackSize:    1,
		PriceCents:  500, // $5/mo per seat
		MaxTotal:    -1,
	},
	AddonCronSchedules: {
		Type:        AddonCronSchedules,
		DisplayName: "Cron Schedules",
		PackSize:    25,
		PriceCents:  500, // $5/mo
		MaxTotal:    -1,
	},
	AddonDataRetention: {
		Type:        AddonDataRetention,
		DisplayName: "Data Retention",
		PackSize:    30,   // +30 days
		PriceCents:  1000, // $10/mo
		MaxTotal:    90,   // max 90 days total
	},
	AddonWebhookEndpoints: {
		Type:        AddonWebhookEndpoints,
		DisplayName: "Webhook Endpoints",
		PackSize:    5,
		PriceCents:  500, // $5/mo
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
		// New canonical addons.
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

		// Deprecated addons (Phase 2b removes these branches with the constants).
		case AddonConcurrentRuns:
			if result.MaxConcurrentRuns != -1 {
				result.MaxConcurrentRuns += increment
			}
		case AddonMembers:
			if result.MaxMembersPerOrg != -1 {
				result.MaxMembersPerOrg += increment
			}
		case AddonCronSchedules:
			if result.MaxScheduledJobs != -1 {
				result.MaxScheduledJobs += increment
			}
		case AddonDataRetention:
			if result.RetentionDays > 0 {
				result.RetentionDays += increment
				if pack.MaxTotal > 0 && result.RetentionDays > pack.MaxTotal {
					result.RetentionDays = pack.MaxTotal
				}
			}
		case AddonWebhookEndpoints:
			if result.MaxWebhookEndpoints != -1 {
				result.MaxWebhookEndpoints += increment
			}
		}
	}

	return result
}
