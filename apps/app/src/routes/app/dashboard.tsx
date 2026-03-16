import { Shell } from "@strait/ui/components/shell";
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
import {
  ActivityIcon,
  AlertIcon,
  CheckCircleIcon,
  ClockIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

export const Route = createFileRoute("/app/dashboard")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <Shell>
      {/* Row 1: Metrics */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard
          change={{ value: 12.5, label: "vs last 24h" }}
          chartColor={CHART_COLORS.active}
          chartData={[42, 28, 65, 89, 74, 56, 48]}
          icon={ActivityIcon}
          title="Total Runs"
          value="2,847"
        />
        <MetricsCard
          change={{ value: 2.1, label: "vs last 24h" }}
          chartColor={CHART_COLORS.success}
          chartData={[91, 93, 92, 95, 94, 96, 94]}
          icon={CheckCircleIcon}
          title="Success Rate"
          value="94.2%"
        />
        <MetricsCard
          change={{ value: -8.3, label: "vs last 24h" }}
          chartColor={CHART_COLORS.error}
          chartData={[3, 1, 5, 7, 4, 2, 3]}
          icon={AlertIcon}
          title="Failed Runs"
          value="168"
        />
        <MetricsCard
          change={{ value: -15.0, label: "vs last hour" }}
          chartColor={CHART_COLORS.neutral}
          chartData={[8, 4, 15, 28, 22, 12, 6]}
          icon={ClockIcon}
          title="Queued"
          value="23"
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
