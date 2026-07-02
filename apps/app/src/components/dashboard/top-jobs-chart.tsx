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
import { TrendingUpIcon } from "@/lib/icons";

const CHART_CONFIG = {
  runs: { label: "Executions", color: "chart-3" },
} satisfies ChartConfig;

const TopJobsChart = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const chartData = (analytics?.slowest_jobs ?? [])
    .map((j) => ({
      job: j.job_slug || j.job_id.slice(0, 12),
      runs: j.total_runs,
    }))
    .slice(0, 5);

  const isEmpty = !hasProject || chartData.length === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Top jobs by execution count
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          {isEmpty ? (
            <ChartEmptyState
              icon={TrendingUpIcon}
              message={
                hasProject
                  ? "No job executions yet. Your top jobs will appear here."
                  : "Create a project to see top jobs."
              }
            />
          ) : (
            <BarChart
              config={CHART_CONFIG}
              containerHeight={240}
              data={chartData}
              dataKey="job"
              layout="vertical"
              legend={false}
              yAxisProps={{ width: 100 }}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default TopJobsChart;
