import { BarList } from "@strait/ui/components/bar-list";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { DonutChart } from "@strait/ui/components/charts";
import { useQuery } from "@tanstack/react-query";
import { analyticsQueryOptions } from "@/hooks/api/use-dashboard";
import { CheckCircleIcon } from "@/lib/icons";

const CHART_CONFIG = {
  Completed: { label: "Completed", color: "chart-1" },
  Failed: { label: "Failed", color: "chart-2" },
  "Timed out": { label: "Timed out", color: "chart-5" },
  Canceled: { label: "Canceled", color: "chart-5" },
} satisfies ChartConfig;

const StatusDistributionChart = ({
  hasProject = true,
}: {
  hasProject?: boolean;
}) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const throughput = analytics?.throughput;
  const chartData = throughput
    ? [
        {
          name: "Completed",
          value: throughput.completed,
        },
        {
          name: "Failed",
          value: throughput.failed,
        },
        {
          name: "Timed out",
          value: throughput.timed_out,
        },
        {
          name: "Canceled",
          value: throughput.canceled,
        },
      ]
    : [];

  const total = chartData.reduce((sum, d) => sum + d.value, 0);
  const isEmpty = !hasProject || total === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Status distribution
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isEmpty ? (
          <div className="h-[240px]">
            <ChartEmptyState
              icon={CheckCircleIcon}
              message={
                hasProject
                  ? "No runs recorded yet. Data will appear after your first job execution."
                  : "Create a project to see status distribution."
              }
            />
          </div>
        ) : (
          <div className="flex items-center gap-6">
            <div className="h-[180px] flex-1">
              <DonutChart
                config={CHART_CONFIG}
                containerHeight={180}
                data={chartData}
                dataKey="value"
                nameKey="name"
                valueFormatter={(value) => value.toLocaleString()}
              />
            </div>
            <BarList
              className="w-56"
              data={chartData}
              sortOrder="none"
              valueFormatter={(value) => {
                const pct =
                  total > 0 ? ((value / total) * 100).toFixed(1) : "0.0";
                return `${value.toLocaleString()} (${pct}%)`;
              }}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default StatusDistributionChart;
