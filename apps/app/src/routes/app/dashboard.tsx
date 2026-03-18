import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { NoProjectState } from "@/components/common/no-project-state";
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
import {
  analyticsQueryOptions as analyticsQueryOptionsFn,
  statsQueryOptions as statsQueryOptionsFn,
} from "@/hooks/api/use-dashboard";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import {
  ActivityIcon,
  AlertIcon,
  CheckCircleIcon,
  ClockIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import type { AuthUser } from "@/routes/__root";

const statsQueryOptions = statsQueryOptionsFn();
const analyticsQueryOptions = analyticsQueryOptionsFn(24);

export const Route = createFileRoute("/app/dashboard")({
  loader: async ({ context }) => {
    const session = (context as unknown as { session: { user: AuthUser } })
      .session;
    const hasProject = !!session?.user?.activeProjectId;
    if (hasProject) {
      await Promise.allSettled([
        context.queryClient.ensureQueryData(statsQueryOptions),
        context.queryClient.ensureQueryData(analyticsQueryOptions),
        context.queryClient.ensureQueryData(runsQueryOptions({ limit: 20 })),
      ]);
    }
    return { hasProject };
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { hasProject } = Route.useLoaderData() as { hasProject: boolean };
  const { session } = Route.useRouteContext() as any;
  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  return <DashboardContent />;
}

function DashboardContent() {
  const { data: stats } = useQuery(statsQueryOptions);
  const { data: analytics } = useQuery(analyticsQueryOptions);

  const queued = stats?.queued ?? 0;
  const executing = stats?.executing ?? 0;
  const delayed = stats?.delayed ?? 0;
  const totalActive = queued + executing + delayed;

  const throughput = analytics?.throughput;
  const health = analytics?.health_summary;
  const totalRuns = throughput
    ? throughput.completed +
      throughput.failed +
      throughput.timed_out +
      throughput.canceled
    : 0;
  const successRate = health?.success_rate ? health.success_rate * 100 : 0;
  const failedRuns = throughput?.failed ?? 0;

  return (
    <Shell>
      {/* Row 1: Metrics */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[]}
          icon={ActivityIcon}
          title="Total Runs (24h)"
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
          value={totalActive.toLocaleString()}
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
