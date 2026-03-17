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
  { time: "00:00", depth: 8 },
  { time: "04:00", depth: 4 },
  { time: "08:00", depth: 15 },
  { time: "12:00", depth: 28 },
  { time: "16:00", depth: 22 },
  { time: "20:00", depth: 12 },
  { time: "24:00", depth: 6 },
];

const LABEL_MAP = {
  depth: {
    label: "Queue depth",
    color: CHART_COLORS.warning,
    format: (v: number) => `${v.toLocaleString()} items`,
  },
};

export function QueueHealthChart() {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Queue Depth Over Time
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
                dataKey="depth"
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
