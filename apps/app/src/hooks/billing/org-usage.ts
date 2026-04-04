/**
 * Organization usage data types and normalization.
 *
 * Defines the shape of the `/v1/usage/current` API response and provides
 * normalization logic to handle optional fields and backward compatibility
 * with deprecated field names.
 */

// PaymentStatus is imported as a type but the API returns plain strings,
// so we keep `string` here for compatibility with Schema.decodeUnknown output.

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
  compute_credit: UsageDimension;
  projects: UsageDimension;
  members: UsageDimension;
  retention_days: number;
  regions_available: number;
};

/** Raw usage dimensions as returned by the API (AI fields may be absent). */
export type RawOrgUsageDimensions = BaseUsageDimensions & {
  ai_model_calls_today?: UsageDimension;
  ai_assistant_messages_today?: UsageDimension;
};

/** Normalized usage dimensions with guaranteed AI fields. */
export type OrgUsageDimensions = BaseUsageDimensions & {
  ai_model_calls_today: UsageDimension;
  ai_assistant_messages_today: UsageDimension;
};

/** Summary of an active addon pack for display in the billing dashboard. */
export type AddonSummary = {
  type: string;
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
  included_credit_microusd: number;
  period_spend_microusd: number;
  overage_microusd: number;
  credit_used_percent: number;
  credit_remaining_microusd: number;
  alerts: UsageAlert[];
  payment_status?: string;
  grace_period_end?: string;
  active_addons?: AddonSummary[];

  /** Enterprise sub-tier identifier (only present for enterprise plans). */
  enterprise_tier?: string;
  /** Enterprise contract end date in "YYYY-MM-DD" format. */
  contract_end_date?: string;
  /** Compute overage discount percentage from the enterprise contract. */
  compute_discount_pct?: number;
  /** SLA uptime percentage from the enterprise contract tier. */
  sla_uptime_pct?: number;
};

/** Normalized org usage data with guaranteed AI fields and enterprise fields carried through. */
export type OrgUsageData = Omit<RawOrgUsageData, "usage"> & {
  usage: OrgUsageDimensions;
};

/** Default empty AI model calls dimension for free tier fallback. */
const EMPTY_AI_MODEL_CALLS: UsageDimension = {
  used: 0,
  limit: 20,
  percent: 0,
  display: "0",
};

/** Default empty usage data returned when no organization is active. */
export const EMPTY_ORG_USAGE: OrgUsageData = {
  org_id: "",
  plan: "free",
  period: { start: "", end: "" },
  included_credit_microusd: 0,
  period_spend_microusd: 0,
  overage_microusd: 0,
  credit_used_percent: 0,
  credit_remaining_microusd: 0,
  usage: {
    runs_today: { used: 0, limit: 5000, percent: 0, display: "0" },
    concurrent_runs: { used: 0, limit: 5, percent: 0, display: "0" },
    compute_credit: {
      used: 0,
      limit: 0,
      percent: 0,
      display: "$0.00 / $0.00",
    },
    projects: { used: 0, limit: 2, percent: 0, display: "0" },
    members: { used: 0, limit: 3, percent: 0, display: "0" },
    ai_model_calls_today: EMPTY_AI_MODEL_CALLS,
    ai_assistant_messages_today: EMPTY_AI_MODEL_CALLS,
    retention_days: 1,
    regions_available: 1,
  },
  alerts: [],
};

/**
 * Normalize raw API usage data into a consistent shape.
 *
 * Handles the `ai_model_calls_today` / `ai_assistant_messages_today` field
 * migration and ensures both fields are always present. Enterprise-specific
 * fields are passed through unchanged.
 *
 * @param raw - The raw response from `/v1/usage/current`.
 * @returns Normalized usage data with guaranteed AI fields.
 */
export const normalizeOrgUsageData = (raw: RawOrgUsageData): OrgUsageData => {
  const aiModelCalls =
    raw.usage.ai_model_calls_today ??
    raw.usage.ai_assistant_messages_today ??
    EMPTY_ORG_USAGE.usage.ai_model_calls_today;

  return {
    ...raw,
    usage: {
      ...raw.usage,
      ai_model_calls_today: aiModelCalls,
      ai_assistant_messages_today:
        raw.usage.ai_assistant_messages_today ?? aiModelCalls,
    },
  };
};
