import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
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

    return await runWithFallback(
      apiEffect<SpendingLimitData>("/v1/spending-limit", {
        params: { org_id: orgId },
      }),
      null
    );
  });

/** Query options for the organization's spending limit and current spend. Refetches every 60s. */
export const spendingLimitQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.spendingLimit.queryKey,
    queryFn: () => getSpendingLimitServerFn(),
    refetchInterval: 60_000,
  });
