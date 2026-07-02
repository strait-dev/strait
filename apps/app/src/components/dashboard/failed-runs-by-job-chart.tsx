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
import { AlertIcon } from "@/lib/icons";

const CHART_CONFIG = {
  failures: { label: "Failures", color: "chart-2" },
} satisfies ChartConfig;

const FailedRunsByJobChart = ({
  hasProject = true,
}: {
  hasProject?: boolean;
}) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const chartData = (analytics?.slowest_jobs ?? [])
    .filter((j) => j.failed_runs > 0)
    .map((j) => ({
      job: j.job_slug || j.job_id.slice(0, 12),
      failures: j.failed_runs,
    }))
    .slice(0, 6);

  const isEmpty = !hasProject || chartData.length === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Failed runs by Job
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={AlertIcon}
              message={
                hasProject
                  ? "No failures in the last 24 hours."
                  : "Create a project to track job failures."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="job"
              legend={false}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default FailedRunsByJobChart;
