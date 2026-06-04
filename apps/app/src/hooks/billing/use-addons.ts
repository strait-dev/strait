/**
 * Addon catalog and active addon utilities.
 *
 * Provides the static addon product catalog and helpers for computing
 * active addon quantities from the usage API response.
 */

import { ACTIVE_ADDONS, formatPrice } from "@strait/billing/products";
import type { AddonSummary } from "@/hooks/billing/org-usage";
import type { ActiveAddonTypeSlug, PlanTierSlug } from "./types";

/** A single addon product in the catalog with pricing and checkout info. */
export type AddonCatalogItem = {
  /** Addon type identifier matching the Go backend. */
  type: ActiveAddonTypeSlug;
  /** Human-readable addon name. */
  name: string;
  /** Short description of what the addon provides. */
  description: string;
  /** Number of units per pack (e.g. 50 concurrent runs). */
  packSize: number;
  /** Unit label for display (e.g. "concurrent runs", "seat"). */
  packUnit: string;
  /** Formatted monthly price string (e.g. "$10/mo"). */
  price: string;
  /** Plans that can buy this add-on. */
  availableOn: PlanTierSlug[];
};

const addonDescription = (type: string): string => {
  switch (type) {
    case "concurrency_100":
      return "Increase the number of orchestration runs that can execute simultaneously.";
    case "history_30d":
      return "Extend run-history retention by 30 days, up to the launch catalog cap.";
    default:
      return "Add five active environments to Pro or Scale plans.";
  }
};

const addonPackUnit = (type: string): string => {
  switch (type) {
    case "concurrency_100":
      return "concurrent runs";
    case "history_30d":
      return "days";
    default:
      return "environments";
  }
};

/** The complete addon product catalog with pricing and checkout slugs. */
export const ADDON_CATALOG: AddonCatalogItem[] = ACTIVE_ADDONS.map((addon) => ({
  type: addon.type,
  name: addon.displayName,
  description: addonDescription(addon.type),
  packSize: addon.packSize,
  packUnit: addonPackUnit(addon.type),
  price: `${formatPrice(addon.priceCents)}/mo`,
  availableOn: addon.availableOn,
}));

export const getAddonCatalogItem = (
  addonType: string
): AddonCatalogItem | undefined =>
  ADDON_CATALOG.find((addon) => addon.type === addonType);

export const isAddonAvailableOnPlan = (
  addonType: string,
  plan: string | undefined
): boolean => {
  const addon = getAddonCatalogItem(addonType);
  return addon?.availableOn.includes((plan ?? "free") as PlanTierSlug) ?? false;
};

export const getAvailableAddonCatalog = (
  plan: string | undefined
): AddonCatalogItem[] =>
  ADDON_CATALOG.filter((addon) =>
    addon.availableOn.includes((plan ?? "free") as PlanTierSlug)
  );

/**
 * Returns the total active pack count for a specific addon type.
 *
 * Sums the quantities of all active addons matching the given type.
 * Returns `0` when no addons are active or the list is undefined.
 *
 * @param activeAddons - The active addons from the usage API response.
 * @param addonType - The addon type to filter by.
 * @returns Total quantity of active packs for the given type.
 */
export const getActivePackCount = (
  activeAddons: AddonSummary[] | undefined,
  addonType: string
): number => {
  if (!activeAddons) {
    return 0;
  }
  return activeAddons
    .filter((a) => a.type === addonType)
    .reduce((sum, a) => sum + a.quantity, 0);
};
