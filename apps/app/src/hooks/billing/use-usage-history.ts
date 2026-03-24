import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
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
    const fromDate = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-01`;
    const toDate = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-${String(now.getUTCDate()).padStart(2, "0")}`;

    return await runWithSentryReport(
      apiEffect<UsageHistoryEntry[]>("/v1/usage/history", {
        params: {
          org_id: orgId,
          from: fromDate,
          to: toDate,
        },
      })
    );
  });

/** Query options for daily usage history in the current billing period. Refetches every 10 minutes. */
export const usageHistoryQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.usageHistory.queryKey,
    queryFn: () => getUsageHistoryServerFn(),
    refetchInterval: 600_000,
    refetchIntervalInBackground: false,
  });
