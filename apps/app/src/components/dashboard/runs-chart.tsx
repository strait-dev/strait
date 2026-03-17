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
  { time: "00:00", completed: 42, failed: 3, executing: 8 },
  { time: "04:00", completed: 28, failed: 1, executing: 5 },
  { time: "08:00", completed: 65, failed: 5, executing: 12 },
  { time: "12:00", completed: 89, failed: 7, executing: 18 },
  { time: "16:00", completed: 74, failed: 4, executing: 14 },
  { time: "20:00", completed: 56, failed: 2, executing: 10 },
  { time: "24:00", completed: 48, failed: 3, executing: 7 },
];

const LABEL_MAP = {
  completed: { label: "Completed", color: CHART_COLORS.success },
  failed: { label: "Failed", color: CHART_COLORS.error },
  executing: { label: "Executing", color: CHART_COLORS.active },
};

const LEGEND_ITEMS = [
  { label: "Completed", color: CHART_COLORS.success },
  { label: "Failed", color: CHART_COLORS.error },
  { label: "Executing", color: CHART_COLORS.active },
];

export function RunsChart() {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">Run Activity</CardTitle>
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
                dataKey="executing"
                fill={CHART_COLORS.active}
                radius={[2, 2, 0, 0]}
                stackId="runs"
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
