package billing

import (
	"context"
	"fmt"
	"time"
)

// AddonType identifies an add-on product.
type AddonType string

const (
	AddonConcurrency100    AddonType = "concurrency_100"
	AddonHistory30d        AddonType = "history_30d"
	AddonComplianceArchive AddonType = "compliance_archive"
	AddonDedicatedWorkers  AddonType = "dedicated_workers"
	AddonEnvironments5     AddonType = "environments_5"
)

// AllAddonTypes returns all known add-on types.
func AllAddonTypes() []AddonType {
	return append([]AddonType(nil), AddonCatalogOrder...)
}

// IsValidAddonType returns true if the addon type is recognized.
func IsValidAddonType(t AddonType) bool {
	_, ok := AddonCatalogs[t]
	return ok
}

func IsLaunchActiveAddonType(t AddonType) bool {
	c, ok := AddonCatalogs[t]
	return ok && c.Status == "active"
}

// AddonPackDefinition describes the increment and pricing for an add-on pack.
type AddonPackDefinition struct {
	Type        AddonType
	DisplayName string
	LookupKey   string // Stripe lookup_key for launch-active add-ons; empty for roadmap
	PackSize    int    // units per pack (e.g. +50 concurrent runs)
	PriceCents  int    // monthly price in cents; zero for roadmap
	MaxTotal    int    // catalog maximum total; -1 = no cap
}

// AddonPacks defines the available add-on packs.
var AddonPacks = addonPacksFromCatalog()

func addonPacksFromCatalog() map[AddonType]AddonPackDefinition {
	packs := make(map[AddonType]AddonPackDefinition, len(AddonCatalogs))
	for _, addonType := range AddonCatalogOrder {
		c := AddonCatalogs[addonType]
		packs[addonType] = AddonPackDefinition{
			Type:        c.Type,
			DisplayName: c.DisplayName,
			LookupKey:   c.LookupKey,
			PackSize:    c.PackSize,
			PriceCents:  c.PriceCents,
			MaxTotal:    c.MaxTotal,
		}
	}
	return packs
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
	appliedPacks := make(map[AddonType]int, len(addons))

	for _, addon := range addons {
		if !addon.Active || addon.Quantity <= 0 {
			continue
		}

		pack, ok := AddonPacks[addon.AddonType]
		if !ok {
			continue
		}

		quantity := allowedAddonQuantity(base, addon, appliedPacks)
		if quantity <= 0 {
			continue
		}
		appliedPacks[addon.AddonType] += quantity
		increment := pack.PackSize * quantity

		switch addon.AddonType {
		case AddonConcurrency100:
			if result.MaxConcurrentRuns != -1 {
				result.MaxConcurrentRuns += increment
			}
		case AddonHistory30d:
			if result.RetentionDays > 0 {
				result.RetentionDays += increment
			}
		case AddonComplianceArchive:
			// Compliance archive remains roadmap at launch until the export
			// pipeline is wired end to end.
		case AddonDedicatedWorkers:
			// Dedicated worker pools remain roadmap at launch.
		case AddonEnvironments5:
			if result.MaxEnvironments != -1 {
				result.MaxEnvironments += increment
			}
		}
	}

	return result
}

func allowedAddonQuantity(base OrgPlanLimits, addon Addon, applied map[AddonType]int) int {
	if base.MaxAddonPacks == nil {
		return 0
	}
	maxPacks, ok := base.MaxAddonPacks[addon.AddonType]
	if !ok {
		return 0
	}
	quantity := addon.Quantity
	if maxPacks >= 0 {
		remaining := maxPacks - applied[addon.AddonType]
		if remaining <= 0 {
			return 0
		}
		if quantity > remaining {
			quantity = remaining
		}
	}
	if pack, ok := AddonPacks[addon.AddonType]; ok && pack.MaxTotal >= 0 {
		switch addon.AddonType {
		case AddonHistory30d:
			if base.RetentionDays <= 0 {
				return quantity
			}
			remainingDays := pack.MaxTotal - base.RetentionDays - applied[addon.AddonType]*pack.PackSize
			if remainingDays <= 0 {
				return 0
			}
			maxQuantity := remainingDays / pack.PackSize
			if quantity > maxQuantity {
				quantity = maxQuantity
			}
		default:
			remainingTotal := pack.MaxTotal - applied[addon.AddonType]
			if remainingTotal <= 0 {
				return 0
			}
			if quantity > remainingTotal {
				quantity = remainingTotal
			}
		}
	}
	return quantity
}

// ReconcileActiveAddonsForPlan deactivates active add-on rows that are no
// longer allowed by the supplied base plan limits or that exceed the plan's
// pack cap. Call this after immediate plan changes so stale paid add-ons do
// not remain authoritative in future entitlement refreshes.
func ReconcileActiveAddonsForPlan(ctx context.Context, store Store, orgID string, limits OrgPlanLimits) (int, error) {
	if store == nil || orgID == "" {
		return 0, nil
	}
	addons, err := store.ListActiveAddons(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("list active addons for reconcile: %w", err)
	}

	applied := make(map[AddonType]int, len(addons))
	deactivated := 0
	for _, addon := range addons {
		quantity := allowedAddonQuantity(limits, addon, applied)
		if quantity <= 0 || quantity < addon.Quantity {
			if err := store.DeactivateAddon(ctx, addon.ID); err != nil {
				return deactivated, fmt.Errorf("deactivate disallowed addon %s: %w", addon.ID, err)
			}
			deactivated++
			continue
		}
		applied[addon.AddonType] += quantity
	}
	return deactivated, nil
}
