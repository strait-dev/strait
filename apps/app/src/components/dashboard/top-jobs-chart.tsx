import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { useQuery } from "@tanstack/react-query";
import { Bar, BarChart, CartesianGrid, Tooltip, XAxis, YAxis } from "recharts";
import { analyticsQueryOptions } from "@/hooks/api/use-dashboard";
import { TrendingUpIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import ChartTooltip from "./chart-tooltip";
import ResponsiveChartContainer from "./responsive-chart-container";

const LABEL_MAP = {
  runs: { label: "Executions", color: CHART_COLORS.active },
};

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
          Top Jobs by Execution Count
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
            <ResponsiveChartContainer
              height="100%"
              minHeight={1}
              minWidth={1}
              width="100%"
            >
              <BarChart data={chartData} layout="vertical">
                <CartesianGrid
                  className="stroke-border"
                  strokeDasharray="3 3"
                />
                <XAxis
                  className="text-muted-foreground"
                  tick={{ fontSize: 14 }}
                  type="number"
                />
                <YAxis
                  className="text-muted-foreground"
                  dataKey="job"
                  tick={{ fontSize: 14 }}
                  type="category"
                  width={100}
                />
                <Tooltip
                  content={<ChartTooltip labelMap={LABEL_MAP} />}
                  cursor={{ fill: "var(--muted)" }}
                />
                <Bar
                  dataKey="runs"
                  fill={CHART_COLORS.active}
                  radius={[0, 4, 4, 0]}
                />
              </BarChart>
            </ResponsiveChartContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default TopJobsChart;
