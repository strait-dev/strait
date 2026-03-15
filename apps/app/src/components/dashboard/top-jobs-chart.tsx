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
  { job: "payment-sync", runs: 412 },
  { job: "email-digest", runs: 356 },
  { job: "inventory-check", runs: 298 },
  { job: "report-gen", runs: 245 },
  { job: "data-export", runs: 189 },
];

const LABEL_MAP = {
  runs: { label: "Executions", color: CHART_COLORS.active },
};

export function TopJobsChart() {
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
            <BarChart data={MOCK_DATA} layout="vertical">
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
