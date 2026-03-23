import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

type CostAlternative = {
  preset: string;
  cost: number;
  savings_pct: number;
};

type CreditInfo = {
  remaining_credit: number;
  estimated_runs_remaining: number;
};

export type CostEstimate = {
  preset: string;
  timeout_secs: number;
  estimated_cost_microusd: number;
  alternatives: CostAlternative[];
  credit_info: CreditInfo;
};

type CostEstimateInput = {
  preset: string;
  timeoutSecs: number;
};

const getCostEstimateServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: CostEstimateInput) =>
    z
      .object({
        preset: z.string().min(1),
        timeoutSecs: z.number().int().positive(),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithSentryReport(
      apiEffect<CostEstimate>("/v1/cost-estimate", {
        params: {
          org_id: orgId,
          preset: data.preset,
          timeout_secs: data.timeoutSecs,
        },
      })
    );
  });

export const costEstimateQueryOptions = (preset: string, timeoutSecs: number) =>
  queryOptions({
    queryKey: queryKeys.billing.costEstimate(preset, timeoutSecs).queryKey,
    queryFn: () => getCostEstimateServerFn({ data: { preset, timeoutSecs } }),
    enabled: !!preset && timeoutSecs > 0,
    staleTime: 30_000,
  });
