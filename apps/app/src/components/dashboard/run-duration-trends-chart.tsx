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
import { ClockIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import ChartEmptyState from "./chart-empty-state";
import ChartTooltip from "./chart-tooltip";

const formatSeconds = (v: number) => `${v.toFixed(1)}s`;

const LABEL_MAP = {
  avg: {
    label: "Average",
    color: CHART_COLORS.active,
    format: formatSeconds,
  },
  p95: {
    label: "P95",
    color: CHART_COLORS.warning,
    format: formatSeconds,
  },
};

const LEGEND_ITEMS = [
  { label: "Average", color: CHART_COLORS.active },
  { label: "P95", color: CHART_COLORS.warning },
];

const RunDurationTrendsChart = ({
  hasProject = true,
}: {
  hasProject?: boolean;
}) => {
  const { data: analytics } = useQuery({
    ...analyticsQueryOptions(24),
    enabled: hasProject,
  });

  const chartData = (analytics?.slowest_jobs ?? [])
    .map((j) => ({
      job: j.job_slug || j.job_id.slice(0, 12),
      avg: j.avg_duration_secs,
      p95: j.p95_duration_secs,
    }))
    .slice(0, 7);

  const isEmpty = !hasProject || chartData.length === 0;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">
          Run Duration by Job
        </CardTitle>
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
              icon={ClockIcon}
              message={
                hasProject
                  ? "No duration data yet. Run some jobs to see timing trends."
                  : "Create a project to track run durations."
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
                  tick={{ fontSize: 11 }}
                />
                <YAxis
                  className="text-muted-foreground"
                  tick={{ fontSize: 14 }}
                  unit="s"
                />
                <Tooltip
                  content={<ChartTooltip labelMap={LABEL_MAP} />}
                  cursor={{ fill: "var(--muted)" }}
                />
                <Bar
                  dataKey="avg"
                  fill={CHART_COLORS.active}
                  radius={[2, 2, 0, 0]}
                />
                <Bar
                  dataKey="p95"
                  fill={CHART_COLORS.warning}
                  radius={[2, 2, 0, 0]}
                />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default RunDurationTrendsChart;
