import { CHART_COLORS } from "@strait/ui/components/chart";
import { MetricCard } from "@strait/ui/components/metric-card";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import InlineError from "@/components/common/inline-error";
import { QueryErrorBoundary } from "@/components/common/query-error-boundary";
import FailedRunsByJobChart from "@/components/dashboard/failed-runs-by-job-chart";
import LiveActivityFeed from "@/components/dashboard/live-activity-feed";
import ProjectCostCard from "@/components/dashboard/project-cost-card";
import QueueHealthChart from "@/components/dashboard/queue-health-chart";
import RecentRunsTable from "@/components/dashboard/recent-runs-table";
import RunDurationTrendsChart from "@/components/dashboard/run-duration-trends-chart";
import RunsChart from "@/components/dashboard/runs-chart";
import StatusDistributionChart from "@/components/dashboard/status-distribution-chart";
import ThroughputChart from "@/components/dashboard/throughput-chart";
import TopJobsChart from "@/components/dashboard/top-jobs-chart";
import {
  analyticsQueryOptions as analyticsQueryOptionsFn,
  statsQueryOptions as statsQueryOptionsFn,
} from "@/hooks/api/use-dashboard";
import {
  ActivityIcon,
  AlertIcon,
  CheckCircleIcon,
  ClockIcon,
} from "@/lib/icons";

const statsQueryOptions = statsQueryOptionsFn();
const analyticsQueryOptions = analyticsQueryOptionsFn(24);

const DashboardContent = ({
  hasProject,
  activeProjectId,
}: {
  hasProject: boolean;
  activeProjectId: string | null;
}) => {
  const { data: stats } = useQuery({
    ...statsQueryOptions,
    enabled: hasProject,
  });
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions,
    enabled: hasProject,
  });

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
      <h1 className="sr-only">Dashboard</h1>
      <h2 className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
        Overview
      </h2>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricCard
          color={CHART_COLORS["chart-3"]}
          icon={ActivityIcon}
          title="Total runs (24h)"
          value={totalRuns.toLocaleString()}
        />
        <MetricCard
          color={CHART_COLORS["chart-1"]}
          icon={CheckCircleIcon}
          title="Success rate"
          value={`${successRate.toFixed(1)}%`}
        />
        <MetricCard
          color={CHART_COLORS["chart-2"]}
          icon={AlertIcon}
          title="Failed runs"
          value={failedRuns.toLocaleString()}
        />
        <MetricCard
          color={CHART_COLORS["chart-5"]}
          icon={ClockIcon}
          title="Queued"
          value={totalActive.toLocaleString()}
        />
      </div>

      {/* Row 2: Run activity + Status distribution */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RunsChart hasProject={hasProject} />
        </div>
        <StatusDistributionChart hasProject={hasProject} />
      </div>

      {/* Project Cost Card */}
      {activeProjectId && (
        <QueryErrorBoundary
          fallback={({ resetErrorBoundary }) => (
            <InlineError
              message="Failed to load project costs"
              onRetry={resetErrorBoundary}
            />
          )}
        >
          <ProjectCostCard activeProjectId={activeProjectId} />
        </QueryErrorBoundary>
      )}

      {/* Row 3: Failed runs by Job + Duration Trends */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <FailedRunsByJobChart hasProject={hasProject} />
        <RunDurationTrendsChart hasProject={hasProject} />
      </div>

      {/* Row 4: Top Jobs + Throughput + Queue health */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <TopJobsChart hasProject={hasProject} />
        <ThroughputChart hasProject={hasProject} />
        <QueueHealthChart hasProject={hasProject} />
      </div>

      {/* Row 5: Recent runs + Activity Feed */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RecentRunsTable hasProject={hasProject} />
        </div>
        <LiveActivityFeed hasProject={hasProject} />
      </div>
    </Shell>
  );
};

export default DashboardContent;
