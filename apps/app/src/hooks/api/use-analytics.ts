import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveProjectAccess } from "@/middlewares/require-access";

type CostTrendPoint = {
  date: string;
  amount_cents: number;
  run_count: number;
};

type TopCostEntry = {
  name: string;
  amount_cents: number;
};

type PerformancePoint = {
  date: string;
  success_rate: number;
  total_runs: number;
};

type CostTrendResponse = {
  period: string;
  spend_microusd: number;
  run_count: number;
};

type TopCostResponse = {
  name: string;
  cost_microusd: number;
};

type PerformanceResponse = {
  throughput?: {
    completed?: number;
    failed?: number;
    timed_out?: number;
    canceled?: number;
  };
  health_summary?: {
    success_rate?: number;
  };
};

type AnalyticsWindow = { window?: string };

const MICRO_USD_PER_CENT = 10_000;
const WINDOW_DAYS: Record<string, number> = {
  "7d": 7,
  "30d": 30,
  "90d": 90,
};

function windowDays(window = "30d") {
  return WINDOW_DAYS[window] ?? WINDOW_DAYS["30d"];
}

function rangeParams(window?: string) {
  const to = new Date();
  const from = new Date(to);
  from.setUTCDate(from.getUTCDate() - windowDays(window));
  return {
    from: from.toISOString(),
    to: to.toISOString(),
  };
}

function periodHours(window?: string) {
  return windowDays(window) * 24;
}

function microUsdToCents(value: number) {
  return value / MICRO_USD_PER_CENT;
}

export const fetchCostTrends = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<CostTrendPoint[]> => {
    await requireActiveProjectAccess(context);
    const response = await runWithSentryReport(
      apiEffect<CostTrendResponse[]>("/v1/analytics/costs/trends", {
        params: rangeParams(data.window),
      })
    );
    return response.map((point) => ({
      date: point.period,
      amount_cents: microUsdToCents(point.spend_microusd),
      run_count: point.run_count,
    }));
  });

export const fetchTopCosts = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<TopCostEntry[]> => {
    await requireActiveProjectAccess(context);
    const response = await runWithSentryReport(
      apiEffect<TopCostResponse[]>("/v1/analytics/costs/top", {
        params: rangeParams(data.window),
      })
    );
    return response.map((item) => ({
      name: item.name,
      amount_cents: microUsdToCents(item.cost_microusd),
    }));
  });

export const fetchPerformance = createServerFn({ method: "GET" })
  .inputValidator((data: AnalyticsWindow) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<PerformancePoint[]> => {
    await requireActiveProjectAccess(context);
    const response = await runWithSentryReport(
      apiEffect<PerformanceResponse>("/v1/analytics/performance", {
        params: { period_hours: periodHours(data.window) },
      })
    );
    const throughput = response.throughput;
    const successRate = response.health_summary?.success_rate ?? 0;
    return [
      {
        date: data.window ?? "30d",
        success_rate: successRate <= 1 ? successRate * 100 : successRate,
        total_runs:
          (throughput?.completed ?? 0) +
          (throughput?.failed ?? 0) +
          (throughput?.timed_out ?? 0) +
          (throughput?.canceled ?? 0),
      },
    ];
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

export type { CostTrendPoint, PerformancePoint, TopCostEntry };
