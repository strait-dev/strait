import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  PerformanceAnalytics,
  QueueStatsResponse,
} from "@/hooks/api/types";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveProjectAccess } from "@/middlewares/require-access";

const fetchStats = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async ({ context }) => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<QueueStatsResponse>("/v1/stats")
    );
  });

const fetchAnalytics = createServerFn({ method: "GET" })
  .inputValidator((data: { periodHours?: number }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<PerformanceAnalytics>("/v1/analytics/performance", {
        params: { period_hours: data.periodHours },
      })
    );
  });

export const statsQueryOptions = () =>
  queryOptions({
    queryKey: ["stats"],
    queryFn: () => fetchStats(),
    staleTime: 60_000,
  });

export const analyticsQueryOptions = (periodHours = 24) =>
  queryOptions({
    queryKey: ["analytics", { periodHours }],
    queryFn: () => fetchAnalytics({ data: { periodHours } }),
    staleTime: 60_000,
  });
