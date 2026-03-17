import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { FailedRunsByJobChart } from "@/components/dashboard/failed-runs-by-job-chart";
import { LiveActivityFeed } from "@/components/dashboard/live-activity-feed";
import { MetricsCard } from "@/components/dashboard/metrics-card";
import { QueueHealthChart } from "@/components/dashboard/queue-health-chart";
import { RecentRunsTable } from "@/components/dashboard/recent-runs-table";
import { RunDurationTrendsChart } from "@/components/dashboard/run-duration-trends-chart";
import { RunsChart } from "@/components/dashboard/runs-chart";
import { StatusDistributionChart } from "@/components/dashboard/status-distribution-chart";
import { ThroughputChart } from "@/components/dashboard/throughput-chart";
import { TopJobsChart } from "@/components/dashboard/top-jobs-chart";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { fetchStats } from "@/lib/api";
import {
  ActivityIcon,
  AlertIcon,
  CheckCircleIcon,
  ClockIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

const statsQueryOptions = {
  queryKey: ["stats"],
  queryFn: () => fetchStats(),
  staleTime: 60_000,
};

export const Route = createFileRoute("/app/dashboard")({
  loader: async ({ context }) => {
    await Promise.allSettled([
      context.queryClient.ensureQueryData(statsQueryOptions),
      context.queryClient.ensureQueryData(runsQueryOptions({ limit: 20 })),
    ]);
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { data: stats } = useQuery(statsQueryOptions);

  const totalRuns = Number(stats?.total_runs ?? 0);
  const successRate = Number(stats?.success_rate ?? 0);
  const failedRuns = Number(stats?.failed_runs ?? 0);
  const queuedRuns = Number(stats?.queued_runs ?? 0);

  return (
    <Shell>
      {/* Row 1: Metrics */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[]}
          icon={ActivityIcon}
          title="Total Runs"
          value={totalRuns.toLocaleString()}
        />
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[]}
          icon={CheckCircleIcon}
          title="Success Rate"
          value={`${successRate.toFixed(1)}%`}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[]}
          icon={AlertIcon}
          title="Failed Runs"
          value={failedRuns.toLocaleString()}
        />
        <MetricsCard
          chartColor={CHART_COLORS.neutral}
          chartData={[]}
          icon={ClockIcon}
          title="Queued"
          value={queuedRuns.toLocaleString()}
        />
      </div>

      {/* Row 2: Run Activity + Status Distribution */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RunsChart />
        </div>
        <StatusDistributionChart />
      </div>

      {/* Row 3: Failed Runs by Job + Duration Trends */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <FailedRunsByJobChart />
        <RunDurationTrendsChart />
      </div>

      {/* Row 4: Top Jobs + Throughput + Queue Health */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <TopJobsChart />
        <ThroughputChart />
        <QueueHealthChart />
      </div>

      {/* Row 5: Recent Runs + Activity Feed */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RecentRunsTable />
        </div>
        <LiveActivityFeed />
      </div>
    </Shell>
  );
}
