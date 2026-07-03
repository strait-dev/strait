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
  count: {
    label: "Runs",
    color: "chart-1",
  },
} satisfies ChartConfig;

const formatRuns = (value: number) => `${value.toLocaleString()} runs`;

const ThroughputChart = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const throughput = analytics?.throughput;
  const chartData = throughput
    ? [
        { status: "Completed", count: throughput.completed },
        { status: "Failed", count: throughput.failed },
        { status: "Timed out", count: throughput.timed_out },
        { status: "Canceled", count: throughput.canceled },
      ]
    : [];

  const total = chartData.reduce((sum, d) => sum + d.count, 0);
  const isEmpty = !hasProject || total === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Throughput (24h)</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={ActivityIcon}
              message={
                hasProject
                  ? "No throughput data yet. Execute jobs to see metrics."
                  : "Create a project to track throughput."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="status"
              legend={false}
              valueFormatter={formatRuns}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default ThroughputChart;
