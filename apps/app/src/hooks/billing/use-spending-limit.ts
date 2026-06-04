/**
 * Spending limit query and mutation hooks.
 *
 * Fetches and updates the organization-level spending limit from
 * `GET/PUT /v1/spending-limit`. Spending limits control whether
 * overage is rejected or the org is just notified.
 */

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { getPostHog } from "@/lib/analytics";
import {
  apiEffect,
  apiEffectWithSchema,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveOrgAccess,
  requireActiveOrgAdmin,
} from "@/middlewares/require-access";
import { SpendingLimitSchema } from "./schemas";
import { type LimitAction, type PlanTierSlug, REFETCH_5M } from "./types";

/** Spending limit and current spend data for the organization. */
export type SpendingLimitData = {
  /** Organization ID. */
  org_id: string;
  /** Current plan tier. */
  plan_tier: PlanTierSlug;
  /** Whether runs beyond the included monthly allowance may enter paid overage. */
  overage_enabled: boolean;
  /** Configured spending limit in USD. */
  spending_limit_usd: number;
  /** Action taken when the limit is reached. */
  limit_action: LimitAction;
  /** Current period spend in USD. */
  current_spend_usd: number;
  /** Overage spend beyond the included allowance in USD. */
  overage_spend_usd: number;
  /** Whether the org is hard-capped (no overage allowed). */
  is_hard_capped: boolean;
};

/** Server function to fetch the organization's spending limit. */
const getSpendingLimitServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    return await runWithSentryReport(
      apiEffectWithSchema("/v1/spending-limit", SpendingLimitSchema, {
        params: { org_id: orgId },
      })
    );
  });

/**
 * Query options for the organization's spending limit and current spend.
 *
 * Refetches every 5 minutes.
 *
 * @returns TanStack Query options for `["billing", "spendingLimit"]`.
 */
export const spendingLimitQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.spendingLimit.queryKey,
    queryFn: () => getSpendingLimitServerFn(),
    refetchInterval: REFETCH_5M,
    refetchIntervalInBackground: false,
  });

/** Input shape for the spending limit update mutation. */
type UpdateSpendingLimitInput = {
  limitMicrousd: number;
  action: string;
  overageEnabled?: boolean;
};

/** Server function to update the organization's spending limit. */
const updateSpendingLimitServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpdateSpendingLimitInput) =>
    z
      .object({
        limitMicrousd: z.number().min(0),
        action: z.string().min(1),
        overageEnabled: z.boolean().optional(),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = await requireActiveOrgAdmin(context);

    return await runWithSentryReport(
      apiEffect<{ status: string }>("/v1/spending-limit", {
        method: "PUT",
        params: { org_id: orgId },
        body: {
          limit_microusd: data.limitMicrousd,
          action: data.action,
          overage_enabled: data.overageEnabled,
        },
      })
    );
  });

/**
 * Mutation hook for updating the organization's spending limit.
 *
 * Invalidates the spending limit query on settlement and tracks
 * the update event via PostHog.
 *
 * @returns A TanStack Query mutation for spending limit updates.
 */
export const useUpdateSpendingLimit = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: UpdateSpendingLimitInput) =>
      updateSpendingLimitServerFn({ data: params }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("spending_limit_updated", {
        new_limit: variables.limitMicrousd,
      });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "spending_limit_updated",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.spendingLimit.queryKey,
      });
    },
  });
};
