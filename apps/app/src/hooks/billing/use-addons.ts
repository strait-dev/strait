/**
 * Addon catalog and active addon utilities.
 *
 * Provides the static addon product catalog and helpers for computing
 * active addon quantities from the usage API response.
 */

import type { AddonSummary } from "@/hooks/billing/org-usage";
import type { AddonTypeSlug } from "./types";

/** A single addon product in the catalog with pricing and checkout info. */
export type AddonCatalogItem = {
  /** Addon type identifier matching the Go backend. */
  type: AddonTypeSlug;
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
  /** Stripe checkout slug for addon purchase. */
  checkoutSlug: string;
};

/** The complete addon product catalog with pricing and checkout slugs. */
export const ADDON_CATALOG: AddonCatalogItem[] = [
  {
    type: "concurrent_runs",
    name: "Concurrent Runs",
    description: "Increase the number of jobs that can execute simultaneously.",
    packSize: 50,
    packUnit: "concurrent runs",
    price: "$10/mo",
    checkoutSlug: "addon-concurrent-runs",
  },
  {
    type: "members",
    name: "Team Members",
    description: "Add more team members to your organization.",
    packSize: 1,
    packUnit: "seat",
    price: "$5/mo",
    checkoutSlug: "addon-members",
  },
  {
    type: "cron_schedules",
    name: "Cron Schedules",
    description: "Add more scheduled job definitions.",
    packSize: 25,
    packUnit: "schedules",
    price: "$5/mo",
    checkoutSlug: "addon-cron-schedules",
  },
  {
    type: "data_retention",
    name: "Data Retention",
    description: "Extend how long job logs and run history are stored.",
    packSize: 30,
    packUnit: "days",
    price: "$10/mo",
    checkoutSlug: "addon-data-retention",
  },
  {
    type: "webhook_endpoints",
    name: "Webhook Endpoints",
    description: "Add more webhook endpoints for real-time notifications.",
    packSize: 5,
    packUnit: "endpoints",
    price: "$5/mo",
    checkoutSlug: "addon-webhook-endpoints",
  },
];

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
