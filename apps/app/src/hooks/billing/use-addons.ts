import type { AddonSummary } from "@/hooks/billing/org-usage";

export type AddonCatalogItem = {
  type: string;
  name: string;
  description: string;
  packSize: number;
  packUnit: string;
  price: string;
  checkoutSlug: string;
};

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

/** Returns the active pack count for an addon type from the API data. */
export function getActivePackCount(
  activeAddons: AddonSummary[] | undefined,
  addonType: string
): number {
  if (!activeAddons) {
    return 0;
  }
  return activeAddons
    .filter((a) => a.type === addonType)
    .reduce((sum, a) => sum + a.quantity, 0);
}
