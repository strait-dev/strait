/**
 * Shared type definitions for the Strait billing system.
 *
 * These types mirror the Go backend's billing domain types and ensure
 * type safety across all billing hooks, server functions, and UI components.
 */

/** Budget or spending limit action determining behavior when a limit is reached. */
export type LimitAction = "reject" | "notify";

/** Plan tier slugs matching the Go backend `domain.PlanTier` constants. */
export type PlanTierSlug =
  | "free"
  | "starter"
  | "pro"
  | "scale"
  | "business"
  | "enterprise";

/** Addon type identifiers matching the Go backend `billing.AddonType` constants. */
export type AddonTypeSlug =
  | "concurrent_runs"
  | "members"
  | "cron_schedules"
  | "data_retention"
  | "webhook_endpoints";

/** Anomaly severity levels returned by the anomaly detection endpoint. */
export type AnomalySeverity = "warning" | "high" | "critical";

/** Downgrade resource impact action. */
export type ResourceAction = "ok" | "reduce" | "remove";

/** Refetch every 5 minutes (300,000 ms). Used for high-traffic billing queries. */
export const REFETCH_5M = 300_000;

/** Refetch every 10 minutes (600,000 ms). Used for lower-traffic billing queries. */
export const REFETCH_10M = 600_000;

/** Stale time of 30 seconds (30,000 ms). Used for cost estimate queries. */
export const STALE_30S = 30_000;

/** All valid plan tier slugs, ordered by rank. */
export const ALL_PLAN_TIERS: readonly PlanTierSlug[] = [
  "free",
  "starter",
  "pro",
  "scale",
  "business",
  "enterprise",
] as const;

/** All valid addon type slugs. */
export const ALL_ADDON_TYPES: readonly AddonTypeSlug[] = [
  "concurrent_runs",
  "members",
  "cron_schedules",
  "data_retention",
  "webhook_endpoints",
] as const;
