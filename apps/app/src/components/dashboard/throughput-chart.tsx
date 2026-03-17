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
  { time: "00:00", throughput: 32 },
  { time: "04:00", throughput: 18 },
  { time: "08:00", throughput: 54 },
  { time: "12:00", throughput: 78 },
  { time: "16:00", throughput: 62 },
  { time: "20:00", throughput: 45 },
  { time: "24:00", throughput: 28 },
];

const LABEL_MAP = {
  throughput: {
    label: "Runs/hour",
    color: CHART_COLORS.success,
    format: (v: number) => `${v.toLocaleString()} runs`,
  },
};

export function ThroughputChart() {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Throughput (runs/hour)
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
                dataKey="throughput"
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
