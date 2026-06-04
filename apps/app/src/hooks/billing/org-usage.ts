/**
 * Organization usage data types and normalization.
 *
 * Defines the shape of the `/v1/usage/current` API response and provides
 * normalization logic for enterprise-specific optional fields.
 */

// PaymentStatus is imported as a type but the API returns plain strings,
// so we keep `string` here for compatibility with Schema.decodeUnknown output.

import type { ActiveAddonTypeSlug } from "./types";

/** A single usage quota dimension with current value, limit, and percentage. */
export type UsageDimension = {
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

/** A usage alert indicating the org is approaching or has exceeded a limit. */
export type UsageAlert = {
  type: string;
  dimension: string;
  threshold: number;
  message: string;
};

/** Base usage dimensions shared between raw and normalized representations. */
type BaseUsageDimensions = {
  runs_today: UsageDimension;
  concurrent_runs: UsageDimension;
  projects: UsageDimension;
  members: UsageDimension;
  retention_days: number;
};

/** Raw usage dimensions as returned by the API. */
export type RawOrgUsageDimensions = BaseUsageDimensions & {
  monthly_runs?: UsageDimension;
};

/** Normalized usage dimensions. */
export type OrgUsageDimensions = BaseUsageDimensions & {
  monthly_runs: UsageDimension;
};

/** Summary of an active addon pack for display in the billing dashboard. */
export type AddonSummary = {
  type: ActiveAddonTypeSlug;
  quantity: number;
};

/** Raw response from `GET /v1/usage/current` before normalization. */
export type RawOrgUsageData = {
  org_id: string;
  plan: string;
  period: {
    start: string;
    end: string;
  };
  usage: RawOrgUsageDimensions;
  period_spend_microusd: number;
  overage_microusd: number;
  alerts: UsageAlert[];
  payment_status?: string;
  grace_period_end?: string;
  active_addons?: AddonSummary[];

  /** Enterprise sub-tier identifier (only present for enterprise plans). */
  enterprise_tier?: string;
  /** Enterprise contract end date in "YYYY-MM-DD" format. */
  contract_end_date?: string;
  /** Overage discount percentage from the enterprise contract. */
  overage_discount_pct?: number;
  /** SLA uptime percentage from the enterprise contract tier. */
  sla_uptime_pct?: number;
};

/** Normalized org usage data with enterprise fields carried through. */
export type OrgUsageData = Omit<RawOrgUsageData, "usage"> & {
  usage: OrgUsageDimensions;
};

/** Default empty usage data returned when no organization is active. */
export const EMPTY_ORG_USAGE: OrgUsageData = {
  org_id: "",
  plan: "free",
  period: { start: "", end: "" },
  period_spend_microusd: 0,
  overage_microusd: 0,
  usage: {
    monthly_runs: { used: 0, limit: 5000, percent: 0, display: "0" },
    runs_today: { used: 0, limit: 5000, percent: 0, display: "0" },
    concurrent_runs: { used: 0, limit: 5, percent: 0, display: "0" },
    projects: { used: 0, limit: 2, percent: 0, display: "0" },
    members: { used: 0, limit: 3, percent: 0, display: "0" },
    retention_days: 1,
  },
  alerts: [],
};

/**
 * Normalize raw API usage data into a consistent shape.
 *
 * Enterprise-specific fields are passed through unchanged.
 *
 * @param raw - The raw response from `/v1/usage/current`.
 * @returns Normalized usage data.
 */
export const normalizeOrgUsageData = (raw: RawOrgUsageData): OrgUsageData => ({
  ...raw,
  usage: {
    ...raw.usage,
    monthly_runs: raw.usage.monthly_runs ?? raw.usage.runs_today,
  },
});
