/**
 * Shared type definitions for the Strait billing system.
 *
 * These types mirror the Go backend's billing domain types and ensure
 * type safety across all billing hooks, server functions, and UI components.
 */

import {
  ACTIVE_ADDON_KEYS,
  type ActiveAddonKey,
  ADDON_KEYS,
  type AddonKey,
  PLAN_KEYS,
  type PlanKey,
} from "@strait/billing/products";

/** Budget or spending limit action determining behavior when a limit is reached. */
export type LimitAction = "reject" | "notify";

/** Plan tier slugs matching the Go backend `domain.PlanTier` constants. */
export type PlanTierSlug = PlanKey;

/** Addon type identifiers matching the Go backend `billing.AddonType` constants. */
export type AddonTypeSlug = AddonKey;

/** Addon type identifiers that are active and sellable at launch. */
export type ActiveAddonTypeSlug = ActiveAddonKey;

/** Anomaly severity levels returned by the anomaly detection endpoint. */
export type AnomalySeverity = "warning" | "high" | "critical";

/** Downgrade resource impact action. */
export type ResourceAction = "ok" | "reduce" | "remove";

/** Role-based access control level. */
export type RBACLevel = "none" | "basic" | "full" | "advanced";

/** Refetch every 5 minutes (300,000 ms). Used for high-traffic billing queries. */
export const REFETCH_5M = 300_000;

/** Refetch every 10 minutes (600,000 ms). Used for lower-traffic billing queries. */
export const REFETCH_10M = 600_000;

/** Stale time of 30 seconds (30,000 ms). Used for cost estimate queries. */
export const STALE_30S = 30_000;

/** All valid plan tier slugs, ordered by rank. */
export const ALL_PLAN_TIERS: readonly PlanTierSlug[] = PLAN_KEYS;

/** All valid addon type slugs. */
export const ALL_ADDON_TYPES: readonly AddonTypeSlug[] = ADDON_KEYS;

/** Addon type slugs accepted in active subscription usage payloads. */
export const ACTIVE_ADDON_TYPES: readonly ActiveAddonTypeSlug[] =
  ACTIVE_ADDON_KEYS;
