import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { BarChart } from "@strait/ui/components/charts";
import { useQuery } from "@tanstack/react-query";
import {
  analyticsQueryOptions,
  statsQueryOptions,
} from "@/hooks/api/use-dashboard";
import { ClockIcon } from "@/lib/icons";

const CHART_CONFIG = {
  count: {
    label: "Count",
    color: "chart-4",
  },
} satisfies ChartConfig;

const formatItems = (value: number) => `${value.toLocaleString()} items`;

const QueueHealthChart = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data: stats } = useQuery({
    ...statsQueryOptions(),
    enabled: hasProject,
  });
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const health = analytics?.health_summary;
  const chartData = [
    { metric: "Queued", count: stats?.queued ?? 0 },
    { metric: "Executing", count: stats?.executing ?? 0 },
    { metric: "Delayed", count: stats?.delayed ?? 0 },
    { metric: "Active jobs", count: health?.active_jobs ?? 0 },
    { metric: "Total jobs", count: health?.total_jobs ?? 0 },
  ];

  const total = chartData.reduce((sum, d) => sum + d.count, 0);
  const isEmpty = !hasProject || total === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Queue health</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={ClockIcon}
              message={
                hasProject
                  ? "No queue activity yet. Jobs will appear here once created."
                  : "Create a project to monitor queue health."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="metric"
              legend={false}
              valueFormatter={formatItems}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default QueueHealthChart;
