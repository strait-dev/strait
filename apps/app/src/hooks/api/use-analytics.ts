import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

type CostTrendPoint = {
  date: string;
  amount_cents: number;
};

type TopCostEntry = {
  project_id: string;
  project_name: string;
  amount_cents: number;
};

type PerformancePoint = {
  date: string;
  success_rate: number;
  total_runs: number;
};

type ComputePoint = {
  date: string;
  compute_seconds: number;
  run_count: number;
};

type AnalyticsWindow = { window?: string };

export const fetchCostTrends = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<CostTrendPoint[]> => {
    return await runWithSentryReport(
      apiEffect<CostTrendPoint[]>("/v1/analytics/costs/trends", {
        params: { window: data.window ?? "30d" },
      })
    );
  });

export const fetchTopCosts = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<TopCostEntry[]> => {
    return await runWithSentryReport(
      apiEffect<TopCostEntry[]>("/v1/analytics/costs/top", {
        params: { window: data.window ?? "30d" },
      })
    );
  });

export const fetchPerformance = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<PerformancePoint[]> => {
    return await runWithSentryReport(
      apiEffect<PerformancePoint[]>("/v1/analytics/performance", {
        params: { window: data.window ?? "30d" },
      })
    );
  });

export const fetchCompute = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<ComputePoint[]> => {
    return await runWithSentryReport(
      apiEffect<ComputePoint[]>("/v1/analytics/compute", {
        params: { window: data.window ?? "30d" },
      })
    );
  });

export const costTrendsQueryOptions = (window = "30d") =>
  queryOptions({
    queryKey: ["analytics", "cost-trends", window],
    queryFn: () => fetchCostTrends({ data: { window } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const topCostsQueryOptions = (window = "30d") =>
  queryOptions({
    queryKey: ["analytics", "top-costs", window],
    queryFn: () => fetchTopCosts({ data: { window } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const performanceQueryOptions = (window = "30d") =>
  queryOptions({
    queryKey: ["analytics", "performance", window],
    queryFn: () => fetchPerformance({ data: { window } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const computeQueryOptions = (window = "30d") =>
  queryOptions({
    queryKey: ["analytics", "compute", window],
    queryFn: () => fetchCompute({ data: { window } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export type { ComputePoint, CostTrendPoint, PerformancePoint, TopCostEntry };
