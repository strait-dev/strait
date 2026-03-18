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
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { fetchAnalytics } from "@/hooks/api/use-dashboard";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  value: { label: "Runs", color: CHART_COLORS.success },
};

export function StatusDistributionChart() {
  const { data: analytics } = useQuery({
    queryKey: ["analytics", { periodHours: 24 }],
    queryFn: () => fetchAnalytics({ data: { periodHours: 24 } }),
    staleTime: 60_000,
  });

  const throughput = analytics?.throughput;
  const chartData = throughput
    ? [
        {
          name: "Completed",
          value: throughput.completed,
          fill: CHART_COLORS.success,
        },
        { name: "Failed", value: throughput.failed, fill: CHART_COLORS.error },
        {
          name: "Timed Out",
          value: throughput.timed_out,
          fill: CHART_COLORS.neutral,
        },
        {
          name: "Canceled",
          value: throughput.canceled,
          fill: CHART_COLORS.neutral,
        },
      ]
    : [];

  const total = chartData.reduce((sum, d) => sum + d.value, 0);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Status Distribution
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-6">
          <div className="h-[180px] flex-1">
            <ResponsiveContainer
              height="100%"
              minHeight={1}
              minWidth={1}
              width="100%"
            >
              <BarChart data={chartData} layout="vertical">
                <XAxis
                  className="text-muted-foreground"
                  tick={{ fontSize: 14 }}
                  type="number"
                />
                <YAxis
                  className="text-muted-foreground"
                  dataKey="name"
                  tick={{ fontSize: 14 }}
                  type="category"
                  width={80}
                />
                <Tooltip
                  content={<ChartTooltip labelMap={LABEL_MAP} />}
                  cursor={{ fill: "var(--muted)" }}
                />
                <Bar dataKey="value" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="flex flex-col gap-2">
            {chartData.map((entry) => {
              const pct =
                total > 0 ? ((entry.value / total) * 100).toFixed(1) : "0.0";
              return (
                <div
                  className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-muted"
                  key={entry.name}
                >
                  <span
                    className="size-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: entry.fill }}
                  />
                  <span className="text-muted-foreground">{entry.name}</span>
                  <span className="ml-auto font-medium tabular-nums">
                    {entry.value.toLocaleString()}
                  </span>
                  <span className="text-muted-foreground">({pct}%)</span>
                </div>
              );
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
