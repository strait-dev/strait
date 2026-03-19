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
import { ChartEmptyState } from "./chart-empty-state";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  completed: { label: "Completed", color: CHART_COLORS.success },
  failed: { label: "Failed", color: CHART_COLORS.error },
  timed_out: { label: "Timed Out", color: CHART_COLORS.warning },
  canceled: { label: "Canceled", color: CHART_COLORS.neutral },
};

const LEGEND_ITEMS = [
  { label: "Completed", color: CHART_COLORS.success },
  { label: "Failed", color: CHART_COLORS.error },
  { label: "Timed Out", color: CHART_COLORS.warning },
  { label: "Canceled", color: CHART_COLORS.neutral },
];

export function RunsChart({ hasProject = true }: { hasProject?: boolean }) {
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
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">Run Activity</CardTitle>
        {!isEmpty && (
          <div className="flex items-center gap-1">
            {LEGEND_ITEMS.map((item) => (
              <div
                className="flex items-center gap-1.5 rounded-md px-2 py-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                key={item.label}
              >
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ backgroundColor: item.color }}
                />
                <span>{item.label}</span>
              </div>
            ))}
          </div>
        )}
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
                  dataKey="period"
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
                  dataKey="completed"
                  fill={CHART_COLORS.success}
                  radius={[2, 2, 0, 0]}
                  stackId="runs"
                />
                <Bar
                  dataKey="failed"
                  fill={CHART_COLORS.error}
                  radius={[0, 0, 0, 0]}
                  stackId="runs"
                />
                <Bar
                  dataKey="timed_out"
                  fill={CHART_COLORS.warning}
                  radius={[0, 0, 0, 0]}
                  stackId="runs"
                />
                <Bar
                  dataKey="canceled"
                  fill={CHART_COLORS.neutral}
                  radius={[2, 2, 0, 0]}
                  stackId="runs"
                />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
