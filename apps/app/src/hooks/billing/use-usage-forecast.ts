import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** Projected usage and cost forecast for the current billing period. */
export type UsageForecastData = {
  projected_monthly_runs: number;
  projected_monthly_compute_usd: number;
  projected_monthly_ai_cost_usd: number;
  recommended_plan: string;
  days_until_limit: number;
};

const getUsageForecastServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithFallback(
      apiEffect<UsageForecastData>("/v1/usage/forecast", {
        params: { org_id: orgId },
      }),
      null
    );
  });

/** Query options for projected monthly usage and cost forecast. Refetches every 5 minutes. */
export const usageForecastQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.usageForecast.queryKey,
    queryFn: () => getUsageForecastServerFn(),
    refetchInterval: 300_000,
  });
