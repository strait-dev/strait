import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Progress } from "@strait/ui/components/progress";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";

import ErrorComponent from "@/components/common/error-component";
import InlineError from "@/components/common/inline-error";
import { QueryErrorBoundary } from "@/components/common/query-error-boundary";
import FailedRunsByJobChart from "@/components/dashboard/failed-runs-by-job-chart";
import LiveActivityFeed from "@/components/dashboard/live-activity-feed";
import MetricsCard from "@/components/dashboard/metrics-card";
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
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { projectBudgetQueryOptions } from "@/hooks/billing/use-project-budget";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { formatMicroUsd } from "@/lib/format";
import {
  ActivityIcon,
  AlertIcon,
  CheckCircleIcon,
  ClockIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import type { AppRouteContext } from "@/routes/app/layout";

const statsQueryOptions = statsQueryOptionsFn();
const analyticsQueryOptions = analyticsQueryOptionsFn(24);

export const Route = createFileRoute("/app/dashboard")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    const activeProjectId = session.user.activeProjectId ?? null;
    if (hasProject) {
      await Promise.allSettled([
        context.queryClient.ensureQueryData(statsQueryOptions),
        context.queryClient.ensureQueryData(analyticsQueryOptions),
        context.queryClient.ensureQueryData(runsQueryOptions({ limit: 20 })),
        context.queryClient.ensureQueryData(projectCostsQueryOptions()),
      ]);
    }
    return { hasProject, activeProjectId };
  },
  errorComponent: ErrorComponent,
  component: RouteComponent,
});

function RouteComponent() {
  const { hasProject, activeProjectId } = Route.useLoaderData();

  return (
    <DashboardContent
      activeProjectId={activeProjectId}
      hasProject={hasProject}
    />
  );
}

function DashboardContent({
  hasProject,
  activeProjectId,
}: {
  hasProject: boolean;
  activeProjectId: string | null;
}) {
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

      {/* Row 3: Failed Runs by Job + Duration Trends */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <FailedRunsByJobChart hasProject={hasProject} />
        <RunDurationTrendsChart hasProject={hasProject} />
      </div>

      {/* Row 4: Top Jobs + Throughput + Queue Health */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <TopJobsChart hasProject={hasProject} />
        <ThroughputChart hasProject={hasProject} />
        <QueueHealthChart hasProject={hasProject} />
      </div>

      {/* Row 5: Recent Runs + Activity Feed */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RecentRunsTable hasProject={hasProject} />
        </div>
        <LiveActivityFeed hasProject={hasProject} />
      </div>
    </Shell>
  );
}

function ProjectCostCard({ activeProjectId }: { activeProjectId: string }) {
  const { data: costs } = useQuery(projectCostsQueryOptions());
  const { data: budget } = useQuery(projectBudgetQueryOptions(activeProjectId));
  const project = costs?.find((c) => c.project_id === activeProjectId);

  if (!project) {
    return null;
  }

  const budgetMicro = budget?.monthly_budget_microusd;
  const hasBudget = budgetMicro !== undefined && budgetMicro > 0;
  const budgetPct = hasBudget
    ? Math.min((project.total_microusd / budgetMicro) * 100, 100)
    : 0;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">
          This Project's Cost
        </CardTitle>
        <Button render={<Link to="/app/billing" />} size="sm" variant="link">
          View Billing
        </Button>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-3 gap-4">
          <div>
            <p className="text-muted-foreground text-xs">Compute</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.compute_microusd)}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground text-xs">AI Cost</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.ai_microusd)}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground text-xs">Total</p>
            <p className="font-medium text-foreground tabular-nums">
              {formatMicroUsd(project.total_microusd)}
            </p>
          </div>
        </div>
        {hasBudget && (
          <div className="mt-3">
            <div className="mb-1 flex items-center justify-between">
              <p className="text-muted-foreground text-xs">
                Budget: {formatMicroUsd(budgetMicro ?? 0)}
              </p>
              <p className="text-muted-foreground text-xs tabular-nums">
                {budgetPct.toFixed(0)}%
              </p>
            </div>
            <Progress className="h-1.5" value={budgetPct} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
