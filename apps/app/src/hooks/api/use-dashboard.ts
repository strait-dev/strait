import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type QueueStatsResponse = {
  queued: number;
  executing: number;
  delayed: number;
};

type JobPerformance = {
  job_id: string;
  job_slug: string;
  avg_duration_secs: number;
  p95_duration_secs: number;
  total_runs: number;
  failed_runs: number;
};

type ThroughputStats = {
  completed: number;
  failed: number;
  timed_out: number;
  canceled: number;
  period_hours: number;
};

type HealthSummary = {
  total_jobs: number;
  active_jobs: number;
  success_rate: number;
  avg_duration_secs: number;
  queue_depth: number;
};

export type PerformanceAnalytics = {
  slowest_jobs: JobPerformance[];
  throughput: ThroughputStats;
  health_summary: HealthSummary;
};

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchStats = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<QueueStatsResponse>("/v1/stats");
  });

export const fetchAnalytics = createServerFn({ method: "GET" })
  .inputValidator((data: { periodHours?: number }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { apiRequest } = await import("@/lib/api-client.server");
    return apiRequest<PerformanceAnalytics>("/v1/analytics/performance", {
      params: { period_hours: data.periodHours },
    });
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

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
