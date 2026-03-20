import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** A single day's usage entry with run counts, compute costs, and AI token usage. */
export type UsageHistoryEntry = {
  date: string;
  runs_count: number;
  compute_cost_microusd: number;
  ai_tokens: number;
  ai_cost_microusd: number;
};

const getUsageHistoryServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return [] as UsageHistoryEntry[];
    }

    const now = new Date();
    const from = new Date(now.getFullYear(), now.getMonth(), 1);
    const to = now;

    return await runWithFallback(
      apiEffect<UsageHistoryEntry[]>("/v1/usage/history", {
        params: {
          org_id: orgId,
          from: from.toISOString().split("T")[0],
          to: to.toISOString().split("T")[0],
        },
      }),
      [] as UsageHistoryEntry[]
    );
  });

/** Query options for daily usage history in the current billing period. Refetches every 5 minutes. */
export const usageHistoryQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.usageHistory.queryKey,
    queryFn: () => getUsageHistoryServerFn(),
    refetchInterval: 300_000,
  });
