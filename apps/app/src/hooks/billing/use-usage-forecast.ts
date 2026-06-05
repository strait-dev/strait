/**
 * Usage forecast query hook.
 *
 * Fetches projected monthly usage and cost from `GET /v1/usage/forecast`.
 * Used by the billing dashboard to show expected end-of-period costs
 * and recommend plan upgrades.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import {
  apiEffectWithSchema,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { UsageForecastSchema } from "./schemas";
import { type PlanTierSlug, REFETCH_10M } from "./types";

/** Projected usage and cost forecast for the current billing period. */
export type UsageForecastData = {
  /** Projected total runs for the month. */
  projected_monthly_runs: number;
  /** Projected run spend in USD. */
  projected_monthly_spend_usd: number;
  /** Recommended plan based on projected usage. */
  recommended_plan: PlanTierSlug;
  /** Estimated days until the current usage limit is exhausted. */
  days_until_limit: number;
  /** Projected overage amount in micro-USD. */
  projected_overage_microusd: number;
  /** Total addon monthly cost in micro-USD. */
  addon_spend_microusd: number;
  /** Whether upgrading to Scale would save money over Pro + overage + addons. */
  scale_breakeven: boolean;
};

/** Server function to fetch the usage forecast from the backend. */
const getUsageForecastServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    return await runWithSentryReport(
      apiEffectWithSchema("/v1/usage/forecast", UsageForecastSchema, {
        params: { org_id: orgId },
      })
    );
  });

/**
 * Query options for projected monthly usage and cost forecast.
 *
 * Refetches every 10 minutes.
 *
 * @returns TanStack Query options for `["billing", "usageForecast"]`.
 */
export const usageForecastQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.usageForecast.queryKey,
    queryFn: () => getUsageForecastServerFn(),
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });
