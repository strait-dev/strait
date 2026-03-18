import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useSuspenseQuery } from "@tanstack/react-query";
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
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  count: {
    label: "Runs",
    color: CHART_COLORS.success,
    format: (v: number) => `${v.toLocaleString()} runs`,
  },
};

export function ThroughputChart() {
  const { data: analytics } = useSuspenseQuery(analyticsQueryOptions(24));

  const throughput = analytics?.throughput;
  const chartData = throughput
    ? [
        { status: "Completed", count: throughput.completed },
        { status: "Failed", count: throughput.failed },
        { status: "Timed Out", count: throughput.timed_out },
        { status: "Canceled", count: throughput.canceled },
      ]
    : [];

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Throughput (24h)</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          <ResponsiveContainer
            height="100%"
            minHeight={1}
            minWidth={1}
            width="100%"
          >
            <BarChart data={chartData}>
              <CartesianGrid className="stroke-border" strokeDasharray="3 3" />
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
        </div>
      </CardContent>
    </Card>
  );
}
