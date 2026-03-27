import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { getPostHog } from "@/lib/analytics";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** Spending limit and current spend data for the organization. */
export type SpendingLimitData = {
  org_id: string;
  plan_tier: string;
  spending_limit_usd: number;
  limit_action: string;
  current_spend_usd: number;
  included_credit_usd: number;
  overage_spend_usd: number;
  is_hard_capped: boolean;
};

const getSpendingLimitServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithSentryReport(
      apiEffect<SpendingLimitData>("/v1/spending-limit", {
        params: { org_id: orgId },
      })
    );
  });

/** Query options for the organization's spending limit and current spend. Refetches every 5 minutes. */
export const spendingLimitQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.spendingLimit.queryKey,
    queryFn: () => getSpendingLimitServerFn(),
    refetchInterval: 300_000,
    refetchIntervalInBackground: false,
  });

type UpdateSpendingLimitInput = {
  limitMicrousd: number;
  action: string;
};

const updateSpendingLimitServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpdateSpendingLimitInput) =>
    z
      .object({
        limitMicrousd: z.number().min(0),
        action: z.string().min(1),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      throw new Error("No active organization");
    }

    return await runWithSentryReport(
      apiEffect<{ status: string }>("/v1/spending-limit", {
        method: "PUT",
        params: { org_id: orgId },
        body: {
          limit_microusd: data.limitMicrousd,
          action: data.action,
        },
      })
    );
  });

export const useUpdateSpendingLimit = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: { limitMicrousd: number; action: string }) =>
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
