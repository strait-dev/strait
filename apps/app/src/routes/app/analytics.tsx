import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import ErrorComponent from "@/components/common/error-component";
import {
  computeQueryOptions,
  costTrendsQueryOptions,
  performanceQueryOptions,
  topCostsQueryOptions,
} from "@/hooks/api/use-analytics";
import { CHART_COLORS } from "@/lib/status-colors";

type AnalyticsWindow = "7d" | "30d" | "90d";

const WINDOWS: { value: AnalyticsWindow; label: string }[] = [
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
];

export const Route = createFileRoute("/app/analytics")({
  errorComponent: ErrorComponent,
  component: AnalyticsPage,
});

function ChartTooltip({
  active,
  payload,
  labelFormatter,
  valueFormatter,
}: {
  active?: boolean;
  payload?: Array<{ name: string; value: number; color: string }>;
  labelFormatter?: (label: string) => string;
  valueFormatter?: (value: number) => string;
}) {
  if (!(active && payload?.length)) {
    return null;
  }
  const item = payload[0];
  return (
    <div className="rounded-lg border border-border bg-popover px-3 py-2 shadow-md">
      <div className="flex items-center gap-2">
        <span
          className="size-2.5 shrink-0 rounded-full"
          style={{ backgroundColor: item.color }}
        />
        <span className="text-muted-foreground text-xs">
          {labelFormatter ? labelFormatter(item.name) : item.name}
        </span>
        <span className="ml-auto font-medium text-popover-foreground text-sm tabular-nums">
          {valueFormatter ? valueFormatter(item.value) : item.value}
        </span>
      </div>
    </div>
  );
}

function AnalyticsPage() {
  const [window, setWindow] = useState<AnalyticsWindow>("30d");

  const { data: costTrends } = useQuery(costTrendsQueryOptions(window));
  const { data: topCosts } = useQuery(topCostsQueryOptions(window));
  const { data: performance } = useQuery(performanceQueryOptions(window));
  const { data: compute } = useQuery(computeQueryOptions(window));

  const costData = costTrends ?? [];
  const topCostData = topCosts ?? [];
  const perfData = performance ?? [];
  const computeData = compute ?? [];

  return (
    <Shell>
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-balance font-normal text-xl tracking-tight sm:text-2xl">
            Analytics
          </h1>
          <p className="text-muted-foreground text-sm">
            Overview of cost, performance, and compute usage.
          </p>
        </div>
        <div className="flex items-center gap-1">
          {WINDOWS.map((w) => (
            <Button
              key={w.value}
              onClick={() => setWindow(w.value)}
              size="sm"
              variant={window === w.value ? "default" : "outline"}
            >
              {w.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Cost Trends */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Cost Trends</CardTitle>
          </CardHeader>
          <CardContent>
            {costData.length > 0 ? (
              <div className="h-[240px]">
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <AreaChart data={costData}>
                    <CartesianGrid
                      className="stroke-border"
                      strokeDasharray="3 3"
                    />
                    <XAxis
                      className="text-muted-foreground"
                      dataKey="date"
                      tick={{ fontSize: 12 }}
                    />
                    <YAxis
                      className="text-muted-foreground"
                      tick={{ fontSize: 12 }}
                      tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
                    />
                    <Tooltip
                      content={
                        <ChartTooltip
                          valueFormatter={(v) => `$${(v / 100).toFixed(2)}`}
                        />
                      }
                      cursor={{
                        stroke: "var(--muted-foreground)",
                        strokeDasharray: "3 3",
                      }}
                    />
                    <Area
                      dataKey="amount_cents"
                      fill={CHART_COLORS.active}
                      fillOpacity={0.15}
                      stroke={CHART_COLORS.active}
                      strokeWidth={2}
                      type="monotone"
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="py-8 text-center text-muted-foreground text-sm">
                No cost data available for this time window.
              </p>
            )}
          </CardContent>
        </Card>

        {/* Top Projects by Cost */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">
              Top Projects by Cost
            </CardTitle>
          </CardHeader>
          <CardContent>
            {topCostData.length > 0 ? (
              <div className="h-[240px]">
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <BarChart data={topCostData} layout="vertical">
                    <CartesianGrid
                      className="stroke-border"
                      strokeDasharray="3 3"
                    />
                    <XAxis
                      className="text-muted-foreground"
                      tick={{ fontSize: 12 }}
                      tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
                      type="number"
                    />
                    <YAxis
                      className="text-muted-foreground"
                      dataKey="project_name"
                      tick={{ fontSize: 12 }}
                      type="category"
                      width={120}
                    />
                    <Tooltip
                      content={
                        <ChartTooltip
                          valueFormatter={(v) => `$${(v / 100).toFixed(2)}`}
                        />
                      }
                      cursor={{ fill: "var(--muted)" }}
                    />
                    <Bar
                      dataKey="amount_cents"
                      fill={CHART_COLORS.warning}
                      radius={[0, 2, 2, 0]}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="py-8 text-center text-muted-foreground text-sm">
                No project cost data available for this time window.
              </p>
            )}
          </CardContent>
        </Card>

        {/* Run Volume */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Run Volume</CardTitle>
          </CardHeader>
          <CardContent>
            {computeData.length > 0 ? (
              <div className="h-[240px]">
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <BarChart data={computeData}>
                    <CartesianGrid
                      className="stroke-border"
                      strokeDasharray="3 3"
                    />
                    <XAxis
                      className="text-muted-foreground"
                      dataKey="date"
                      tick={{ fontSize: 12 }}
                    />
                    <YAxis
                      className="text-muted-foreground"
                      tick={{ fontSize: 12 }}
                    />
                    <Tooltip
                      content={<ChartTooltip />}
                      cursor={{ fill: "var(--muted)" }}
                    />
                    <Bar
                      dataKey="run_count"
                      fill={CHART_COLORS.success}
                      radius={[2, 2, 0, 0]}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="py-8 text-center text-muted-foreground text-sm">
                No run volume data available for this time window.
              </p>
            )}
          </CardContent>
        </Card>

        {/* Performance */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Success Rate</CardTitle>
          </CardHeader>
          <CardContent>
            {perfData.length > 0 ? (
              <div className="h-[240px]">
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <LineChart data={perfData}>
                    <CartesianGrid
                      className="stroke-border"
                      strokeDasharray="3 3"
                    />
                    <XAxis
                      className="text-muted-foreground"
                      dataKey="date"
                      tick={{ fontSize: 12 }}
                    />
                    <YAxis
                      className="text-muted-foreground"
                      domain={[0, 100]}
                      tick={{ fontSize: 12 }}
                      tickFormatter={(v: number) => `${v}%`}
                    />
                    <Tooltip
                      content={
                        <ChartTooltip
                          valueFormatter={(v) => `${v.toFixed(1)}%`}
                        />
                      }
                      cursor={{
                        stroke: "var(--muted-foreground)",
                        strokeDasharray: "3 3",
                      }}
                    />
                    <Line
                      dataKey="success_rate"
                      dot={false}
                      stroke={CHART_COLORS.success}
                      strokeWidth={2}
                      type="monotone"
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="py-8 text-center text-muted-foreground text-sm">
                No performance data available for this time window.
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
