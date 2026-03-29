package billing

import "time"

// AddonType identifies an add-on product.
type AddonType string

const (
	AddonConcurrentRuns   AddonType = "concurrent_runs"
	AddonMembers          AddonType = "members"
	AddonCronSchedules    AddonType = "cron_schedules"
	AddonDataRetention    AddonType = "data_retention"
	AddonWebhookEndpoints AddonType = "webhook_endpoints"
)

// AllAddonTypes returns all known add-on types.
func AllAddonTypes() []AddonType {
	return []AddonType{
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
	case AddonConcurrentRuns, AddonMembers, AddonCronSchedules,
		AddonDataRetention, AddonWebhookEndpoints:
		return true
	}
	return false
}

// AddonPackDefinition describes the increment and pricing for an add-on pack.
type AddonPackDefinition struct {
	Type        AddonType
	DisplayName string
	PackSize    int // units per pack (e.g. +50 concurrent runs)
	PriceCents  int // monthly price in cents
	MaxTotal    int // maximum total after add-ons; -1 = no cap
}

// AddonPacks defines the available add-on packs.
var AddonPacks = map[AddonType]AddonPackDefinition{
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
	ID                  string
	OrgID               string
	AddonType           AddonType
	Quantity            int
	PolarSubscriptionID *string
	Active              bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
