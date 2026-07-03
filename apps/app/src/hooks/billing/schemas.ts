/**
 * Effect Schema definitions for billing API responses.
 *
 * These schemas provide runtime validation of Go backend responses,
 * catching contract drift between the API and frontend types early.
 * Each schema mirrors the corresponding TypeScript type in the billing hooks.
 *
 * @see https://effect.website/docs/schema/introduction
 */

import { ACTIVE_ADDON_KEYS } from "@strait/billing/products";
import { Schema } from "effect";

const ActiveAddonTypeSchema = Schema.Literal(...ACTIVE_ADDON_KEYS);

/**
 * Schema for a single usage quota dimension.
 *
 * @see {@link import("./org-usage").UsageDimension}
 */
const UsageDimensionSchema = Schema.Struct({
  used: Schema.Number,
  limit: Schema.Number,
  percent: Schema.Number,
  display: Schema.optional(Schema.String),
});

/**
 * Schema for a usage alert from the API.
 *
 * @see {@link import("./org-usage").UsageAlert}
 */
const UsageAlertSchema = Schema.Struct({
  type: Schema.String,
  dimension: Schema.String,
  threshold: Schema.Number,
  message: Schema.String,
});

/**
 * Schema for an active addon summary.
 *
 * @see {@link import("./org-usage").AddonSummary}
 */
const AddonSummarySchema = Schema.Struct({
  type: ActiveAddonTypeSchema,
  quantity: Schema.Number,
});

/** Schema for the raw usage dimensions from the API. */
const RawOrgUsageDimensionsSchema = Schema.Struct({
  monthly_runs: Schema.optional(UsageDimensionSchema),
  runs_today: UsageDimensionSchema,
  concurrent_runs: UsageDimensionSchema,
  projects: UsageDimensionSchema,
  members: UsageDimensionSchema,
  retention_days: Schema.Number,
});

/**
 * Schema for the full `/v1/usage/current` API response.
 *
 * Enterprise-specific fields are optional and only present when
 * the organization has an active enterprise contract.
 *
 * @see {@link import("./org-usage").RawOrgUsageData}
 */
export const OrgUsageResponseSchema = Schema.mutable(
  Schema.Struct({
    org_id: Schema.String,
    plan: Schema.String,
    period: Schema.mutable(
      Schema.Struct({
        start: Schema.String,
        end: Schema.String,
      })
    ),
    usage: Schema.mutable(RawOrgUsageDimensionsSchema),
    period_spend_microusd: Schema.Number,
    overage_microusd: Schema.Number,
    alerts: Schema.mutable(Schema.Array(Schema.mutable(UsageAlertSchema))),
    payment_status: Schema.optional(Schema.String),
    grace_period_end: Schema.optional(Schema.String),
    active_addons: Schema.optional(
      Schema.mutable(Schema.Array(Schema.mutable(AddonSummarySchema)))
    ),
    enterprise_tier: Schema.optional(Schema.String),
    contract_end_date: Schema.optional(Schema.String),
    overage_discount_pct: Schema.optional(Schema.Number),
    sla_uptime_pct: Schema.optional(Schema.Number),
  })
);

/**
 * Schema for the `/v1/spending-limit` API response.
 *
 * @see {@link import("./use-spending-limit").SpendingLimitData}
 */
export const SpendingLimitSchema = Schema.Struct({
  org_id: Schema.String,
  plan_tier: Schema.String,
  overage_enabled: Schema.Boolean,
  spending_limit_usd: Schema.Number,
  limit_action: Schema.String,
  current_spend_usd: Schema.Number,
  overage_spend_usd: Schema.Number,
  is_hard_capped: Schema.Boolean,
});

/**
 * Schema for the `/v1/usage/forecast` API response.
 *
 * @see {@link import("./use-usage-forecast").UsageForecastData}
 */
export const UsageForecastSchema = Schema.Struct({
  projected_monthly_runs: Schema.Number,
  projected_monthly_spend_usd: Schema.Number,
  recommended_plan: Schema.String,
  days_until_limit: Schema.Number,
  projected_overage_microusd: Schema.Number,
  addon_spend_microusd: Schema.Number,
  scale_breakeven: Schema.Boolean,
});
