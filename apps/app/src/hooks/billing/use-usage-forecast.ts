import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

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
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return null;
    }

    return await runWithFallback(
      apiEffect<UsageForecastData>("/v1/usage/forecast", {
        params: { org_id: orgId },
      }),
      null
    );
  });

export function useUsageForecast() {
  return useQuery({
    queryKey: ["usage-forecast"],
    queryFn: () => getUsageForecastServerFn(),
    refetchInterval: 300_000,
  });
}
