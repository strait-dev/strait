/**
 * Job cost estimation query hook.
 *
 * Fetches estimated execution cost from `GET /v1/cost-estimate` for a given
 * machine preset and timeout. Used in job creation forms and pricing calculators.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";
import { STALE_30S } from "./types";

/** Cost comparison for an alternative machine preset. */
type CostAlternative = {
  /** Machine preset name. */
  preset: string;
  /** Estimated cost in micro-USD. */
  cost: number;
  /** Percentage savings compared to the selected preset. */
  savings_pct: number;
};

/** Remaining credit information for the organization. */
type CreditInfo = {
  /** Remaining compute credit in micro-USD. */
  remaining_credit: number;
  /** Estimated number of runs remaining within the credit. */
  estimated_runs_remaining: number;
};

/** Cost estimate response from the backend. */
export type CostEstimate = {
  /** Machine preset name (e.g. "micro", "small-1x"). */
  preset: string;
  /** Job timeout in seconds. */
  timeout_secs: number;
  /** Estimated cost for a single run in micro-USD. */
  estimated_cost_microusd: number;
  /** Cost comparisons with alternative presets. */
  alternatives: CostAlternative[];
  /** Current credit balance and estimated runs remaining. */
  credit_info: CreditInfo;
};

/** Input shape for the cost estimate server function. */
type CostEstimateInput = {
  preset: string;
  timeoutSecs: number;
};

/** Server function to fetch the cost estimate from the backend. */
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

/**
 * Query options for job cost estimation.
 *
 * Only enabled when a preset is specified and timeout is positive.
 * Uses a 30-second stale time since cost estimates are deterministic
 * and only change when the org's plan or credit changes.
 *
 * @param preset - Machine preset name (e.g. "micro").
 * @param timeoutSecs - Job timeout in seconds.
 * @returns TanStack Query options for `["billing", "costEstimate", preset, timeoutSecs]`.
 */
export const costEstimateQueryOptions = (preset: string, timeoutSecs: number) =>
  queryOptions({
    queryKey: queryKeys.billing.costEstimate(preset, timeoutSecs).queryKey,
    queryFn: () => getCostEstimateServerFn({ data: { preset, timeoutSecs } }),
    enabled: !!preset && timeoutSecs > 0,
    staleTime: STALE_30S,
  });
