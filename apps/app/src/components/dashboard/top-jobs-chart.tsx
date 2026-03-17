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
import { fetchAnalytics } from "@/lib/api";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  runs: { label: "Executions", color: CHART_COLORS.active },
};

export function TopJobsChart() {
  const { data: analytics } = useQuery({
    queryKey: ["analytics", { periodHours: 24 }],
    queryFn: () => fetchAnalytics({ data: { periodHours: 24 } }),
    staleTime: 60_000,
  });

  const chartData = (analytics?.slowest_jobs ?? [])
    .map((j) => ({
      job: j.job_slug || j.job_id.slice(0, 12),
      runs: j.total_runs,
    }))
    .slice(0, 5);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Top Jobs by Execution Count
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
            <BarChart data={chartData} layout="vertical">
              <CartesianGrid className="stroke-border" strokeDasharray="3 3" />
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
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
