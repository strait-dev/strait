// @ts-nocheck
import { useQuery } from "@tanstack/react-query";

import MetricsCard from "@/components/dashboard/metrics-card";
import {
  analyticsQueryOptions,
  statsQueryOptions,
} from "@/hooks/api/use-dashboard";
import {
  ActivityIcon,
  AlertIcon,
  BriefcaseIcon,
  CalendarIcon,
  CheckCircleIcon,
  ClockIcon,
  WorkflowIcon,
  ZapIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

const OverviewMetrics = () => {
  const { data: stats } = useQuery(statsQueryOptions());
  const { data: analytics } = useQuery(analyticsQueryOptions(24));

  const health = analytics?.health_summary;
  const throughput = analytics?.throughput;

  const totalRuns = throughput
    ? throughput.completed +
      throughput.failed +
      throughput.timed_out +
      throughput.canceled
    : 0;
  const successRate = health?.success_rate ?? 0;
  const failedRuns = throughput?.failed ?? 0;
  const avgDuration = health?.avg_duration_secs ?? 0;

  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={ActivityIcon}
          title="Total Runs (24h)"
          value={totalRuns}
        />
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[0]}
          icon={CheckCircleIcon}
          title="Success Rate"
          value={`${successRate.toFixed(1)}%`}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[0]}
          icon={AlertIcon}
          title="Failed Runs"
          value={failedRuns}
        />
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={ClockIcon}
          title="Avg Duration"
          value={`${avgDuration.toFixed(1)}s`}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[0]}
          icon={BriefcaseIcon}
          title="Active Jobs"
          value={health?.active_jobs ?? 0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={WorkflowIcon}
          title="Workflows"
          value={health?.total_jobs ?? 0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[0]}
          icon={ZapIcon}
          title="Dead Letter"
          value={0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.neutral}
          chartData={[0]}
          description={`Queue depth: ${stats?.queued ?? 0}`}
          icon={CalendarIcon}
          title="Queued"
          value={stats?.queued ?? 0}
        />
      </div>
    </>
  );
};

export default OverviewMetrics;
