import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { Bar, BarChart, CartesianGrid, Tooltip, XAxis, YAxis } from "recharts";
import {
  analyticsQueryOptions,
  statsQueryOptions,
} from "@/hooks/api/use-dashboard";
import { ClockIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import ChartEmptyState from "./chart-empty-state";
import ChartTooltip from "./chart-tooltip";
import ResponsiveChartContainer from "./responsive-chart-container";

const LABEL_MAP = {
  count: {
    label: "Count",
    color: CHART_COLORS.warning,
    format: (v: number) => `${v.toLocaleString()} items`,
  },
};

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
    { metric: "Active Jobs", count: health?.active_jobs ?? 0 },
    { metric: "Total Jobs", count: health?.total_jobs ?? 0 },
  ];

  const total = chartData.reduce((sum, d) => sum + d.count, 0);
  const isEmpty = !hasProject || total === 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Queue Health</CardTitle>
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
            <ResponsiveChartContainer
              height="100%"
              minHeight={1}
              minWidth={1}
              width="100%"
            >
              <BarChart data={chartData}>
                <CartesianGrid
                  className="stroke-border"
                  strokeDasharray="3 3"
                />
                <XAxis
                  className="text-muted-foreground"
                  dataKey="metric"
                  tick={{ fontSize: 14 }}
                />
                <YAxis
                  className="text-muted-foreground"
                  tick={{ fontSize: 14 }}
                />
                <Tooltip
                  content={<ChartTooltip labelMap={LABEL_MAP} />}
                  cursor={{ fill: "var(--muted)" }}
                />
                <Bar
                  dataKey="count"
                  fill={CHART_COLORS.warning}
                  radius={[4, 4, 0, 0]}
                />
              </BarChart>
            </ResponsiveChartContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default QueueHealthChart;
