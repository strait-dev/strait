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
import { analyticsQueryOptions } from "@/hooks/api/use-dashboard";
import { ActivityIcon } from "@/lib/icons";

const CHART_CONFIG = {
  completed: { label: "Completed", color: "chart-1" },
  failed: { label: "Failed", color: "chart-2" },
  timed_out: { label: "Timed out", color: "chart-4" },
  canceled: { label: "Canceled", color: "chart-5" },
} satisfies ChartConfig;

const RunsChart = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const throughput = analytics?.throughput;
  const chartData = throughput
    ? [
        {
          period: "Last 24h",
          completed: throughput.completed,
          failed: throughput.failed,
          timed_out: throughput.timed_out,
          canceled: throughput.canceled,
        },
      ]
    : [];

  const isEmpty = !hasProject || chartData.length === 0 || !throughput;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Run activity</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={ActivityIcon}
              message={
                hasProject
                  ? "No run activity yet. Trigger a job to see data here."
                  : "Create a project to start tracking run activity."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="period"
              type="stacked"
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default RunsChart;
