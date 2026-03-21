import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { analyticsQueryOptions } from "@/hooks/api/use-dashboard";
import { ActivityIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import ChartEmptyState from "./chart-empty-state";
import ChartTooltip from "./chart-tooltip";

const LABEL_MAP = {
  count: {
    label: "Runs",
    color: CHART_COLORS.success,
    format: (v: number) => `${v.toLocaleString()} runs`,
  },
};

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
        { status: "Timed Out", count: throughput.timed_out },
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
            <ResponsiveContainer
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
                  dataKey="status"
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
                  fill={CHART_COLORS.success}
                  radius={[4, 4, 0, 0]}
                />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default ThroughputChart;
