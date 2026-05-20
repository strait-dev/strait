/**
 * Job cost estimation query hook.
 *
 * Estimates execution cost for a given machine preset and timeout.
 * Used in job creation forms and pricing calculators.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { STALE_30S } from "./types";

const COST_PER_RUN_MICROUSD = 20;

const PRESET_MULTIPLIERS: Record<string, number> = {
  micro: 1,
  small: 2,
  "small-1x": 2,
  medium: 4,
  "medium-2x": 4,
  large: 8,
  "large-4x": 8,
};

const estimateCost = (preset: string, timeoutSecs: number): number => {
  const multiplier = PRESET_MULTIPLIERS[preset] ?? 1;
  const timeoutMultiplier = Math.max(1, Math.ceil(timeoutSecs / 300));
  return COST_PER_RUN_MICROUSD * multiplier * timeoutMultiplier;
};

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
    await requireActiveOrgAccess(context);

    const estimatedCost = estimateCost(data.preset, data.timeoutSecs);
    const alternatives = Object.keys(PRESET_MULTIPLIERS)
      .filter((preset) => preset !== data.preset && !preset.includes("-"))
      .map((preset) => {
        const cost = estimateCost(preset, data.timeoutSecs);
        return {
          preset,
          cost,
          savings_pct:
            estimatedCost > 0
              ? Math.max(0, Math.round((1 - cost / estimatedCost) * 100))
              : 0,
        };
      });

    return {
      preset: data.preset,
      timeout_secs: data.timeoutSecs,
      estimated_cost_microusd: estimatedCost,
      alternatives,
      credit_info: {
        remaining_credit: 0,
        estimated_runs_remaining: 0,
      },
    } satisfies CostEstimate;
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
