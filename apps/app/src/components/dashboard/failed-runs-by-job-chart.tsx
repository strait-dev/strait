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
import { AlertIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartEmptyState } from "./chart-empty-state";
import { ChartTooltip } from "./chart-tooltip";

const LABEL_MAP = {
  failures: { label: "Failures", color: CHART_COLORS.error },
};

export function FailedRunsByJobChart({
  hasProject = true,
}: {
  hasProject?: boolean;
}) {
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
          Failed Runs by Job
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
                  dataKey="job"
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
                  dataKey="failures"
                  fill={CHART_COLORS.error}
                  radius={[4, 4, 0, 0]}
                />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
