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
import { fetchAnalytics, fetchStats } from "@/lib/api";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  count: {
    label: "Count",
    color: CHART_COLORS.warning,
    format: (v: number) => `${v.toLocaleString()} items`,
  },
};

export function QueueHealthChart() {
  const { data: stats } = useQuery({
    queryKey: ["stats"],
    queryFn: () => fetchStats(),
    staleTime: 60_000,
  });
  const { data: analytics } = useQuery({
    queryKey: ["analytics", { periodHours: 24 }],
    queryFn: () => fetchAnalytics({ data: { periodHours: 24 } }),
    staleTime: 60_000,
  });

  const health = analytics?.health_summary;
  const chartData = [
    { metric: "Queued", count: stats?.queued ?? 0 },
    { metric: "Executing", count: stats?.executing ?? 0 },
    { metric: "Delayed", count: stats?.delayed ?? 0 },
    { metric: "Active Jobs", count: health?.active_jobs ?? 0 },
    { metric: "Total Jobs", count: health?.total_jobs ?? 0 },
  ];

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Queue Health
        </CardTitle>
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
                dataKey="metric"
                tick={{ fontSize: 12 }}
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
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
