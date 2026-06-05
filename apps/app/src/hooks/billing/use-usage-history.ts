/**
 * Usage history query hook.
 *
 * Fetches daily usage history for the current billing period from
 * `GET /v1/usage/history`. Used by the usage chart in the billing dashboard.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { REFETCH_10M } from "./types";

/** A single day's usage entry with run counts and orchestration costs. */
export type UsageHistoryEntry = {
  /** Date in "YYYY-MM-DD" format. */
  date: string;
  /** Total runs executed on this day. */
  runs_count: number;
  /** Run spend for the day in micro-USD. */
  spend_microusd: number;
};

/** Server function to fetch daily usage history for the current period. */
const getUsageHistoryServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

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

/**
 * Query options for daily usage history in the current billing period.
 *
 * Refetches every 10 minutes.
 *
 * @returns TanStack Query options for `["billing", "usageHistory"]`.
 */
export const usageHistoryQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.usageHistory.queryKey,
    queryFn: () => getUsageHistoryServerFn(),
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });
