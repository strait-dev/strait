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
import { ClockIcon } from "@/lib/icons";

const formatSeconds = (v: number) => `${v.toFixed(1)}s`;

const CHART_CONFIG = {
  avg: {
    label: "Average",
    color: "chart-3",
  },
  p95: {
    label: "P95",
    color: "chart-4",
  },
} satisfies ChartConfig;

const RunDurationTrendsChart = ({
  hasProject = true,
}: {
  hasProject?: boolean;
}) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const chartData = (analytics?.slowest_jobs ?? [])
    .map((j) => ({
      job: j.job_slug || j.job_id.slice(0, 12),
      avg: j.avg_duration_secs,
      p95: j.p95_duration_secs,
    }))
    .slice(0, 7);

  const isEmpty = !hasProject || chartData.length === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Run Duration by Job
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={ClockIcon}
              message={
                hasProject
                  ? "No duration data yet. Run some jobs to see timing trends."
                  : "Create a project to track run durations."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="job"
              valueFormatter={formatSeconds}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default RunDurationTrendsChart;
