import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { CHART_COLORS } from "@/lib/status-colors";
import { ChartTooltip } from "./chart-tooltip";

const MOCK_DATA = [
  { time: "00:00", avg: 1.2, p95: 3.4 },
  { time: "04:00", avg: 1.1, p95: 2.8 },
  { time: "08:00", avg: 1.8, p95: 4.2 },
  { time: "12:00", avg: 2.4, p95: 5.1 },
  { time: "16:00", avg: 1.9, p95: 4.6 },
  { time: "20:00", avg: 1.4, p95: 3.2 },
  { time: "24:00", avg: 1.3, p95: 3.0 },
];

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

export function RunDurationTrendsChart() {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">
          Run Duration Trends
        </CardTitle>
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
      </CardHeader>
      <CardContent>
        <div className="h-[240px]">
          <ResponsiveContainer
            height="100%"
            minHeight={1}
            minWidth={1}
            width="100%"
          >
            <BarChart data={MOCK_DATA}>
              <CartesianGrid className="stroke-border" strokeDasharray="3 3" />
              <XAxis
                className="text-muted-foreground"
                dataKey="time"
                tick={{ fontSize: 14 }}
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
        </div>
      </CardContent>
    </Card>
  );
}
