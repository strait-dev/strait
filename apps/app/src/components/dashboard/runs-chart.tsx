import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

const MOCK_DATA = [
  { time: "00:00", completed: 42, failed: 3, executing: 8 },
  { time: "04:00", completed: 28, failed: 1, executing: 5 },
  { time: "08:00", completed: 65, failed: 5, executing: 12 },
  { time: "12:00", completed: 89, failed: 7, executing: 18 },
  { time: "16:00", completed: 74, failed: 4, executing: 14 },
  { time: "20:00", completed: 56, failed: 2, executing: 10 },
  { time: "24:00", completed: 48, failed: 3, executing: 7 },
];

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
      <span
        className="inline-block size-2 rounded-full"
        style={{ backgroundColor: `var(--color-${color})` }}
      />
      {label}
    </div>
  );
}

export function RunsChart() {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">Run Activity</CardTitle>
        <div className="flex items-center gap-3">
          <LegendDot color="chart-1" label="Completed" />
          <LegendDot color="chart-4" label="Failed" />
          <LegendDot color="chart-3" label="Executing" />
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
            <AreaChart data={MOCK_DATA}>
              <defs>
                <linearGradient id="gradCompleted" x1="0" x2="0" y1="0" y2="1">
                  <stop
                    offset="0%"
                    stopColor="var(--color-chart-1)"
                    stopOpacity={0.3}
                  />
                  <stop
                    offset="100%"
                    stopColor="var(--color-chart-1)"
                    stopOpacity={0}
                  />
                </linearGradient>
                <linearGradient id="gradFailed" x1="0" x2="0" y1="0" y2="1">
                  <stop
                    offset="0%"
                    stopColor="var(--color-chart-4)"
                    stopOpacity={0.3}
                  />
                  <stop
                    offset="100%"
                    stopColor="var(--color-chart-4)"
                    stopOpacity={0}
                  />
                </linearGradient>
                <linearGradient id="gradExecuting" x1="0" x2="0" y1="0" y2="1">
                  <stop
                    offset="0%"
                    stopColor="var(--color-chart-3)"
                    stopOpacity={0.3}
                  />
                  <stop
                    offset="100%"
                    stopColor="var(--color-chart-3)"
                    stopOpacity={0}
                  />
                </linearGradient>
              </defs>
              <CartesianGrid className="stroke-border" strokeDasharray="3 3" />
              <XAxis
                className="text-muted-foreground"
                dataKey="time"
                tick={{ fontSize: 11 }}
              />
              <YAxis
                className="text-muted-foreground"
                tick={{ fontSize: 11 }}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "hsl(var(--popover))",
                  borderColor: "hsl(var(--border))",
                  borderRadius: 8,
                  fontSize: 12,
                }}
              />
              <Area
                dataKey="completed"
                fill="url(#gradCompleted)"
                stroke="var(--color-chart-1)"
                strokeWidth={2}
                type="monotone"
              />
              <Area
                dataKey="failed"
                fill="url(#gradFailed)"
                stroke="var(--color-chart-4)"
                strokeWidth={2}
                type="monotone"
              />
              <Area
                dataKey="executing"
                fill="url(#gradExecuting)"
                stroke="var(--color-chart-3)"
                strokeWidth={2}
                type="monotone"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
