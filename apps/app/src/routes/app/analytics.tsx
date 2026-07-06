import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { AreaChart, BarChart, LineChart } from "@strait/ui/components/charts";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { z } from "zod";
import ErrorComponent from "@/components/common/error-component";
import {
  costTrendsQueryOptions,
  performanceQueryOptions,
  topCostsQueryOptions,
} from "@/hooks/api/use-analytics";
import { seo } from "@/lib/seo";

type AnalyticsWindow = "7d" | "30d" | "90d";

const WINDOWS: { value: AnalyticsWindow; label: string }[] = [
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
];

const COST_TRENDS_CONFIG = {
  amount_cents: { label: "Cost", color: "chart-3" },
} satisfies ChartConfig;

const TOP_COSTS_CONFIG = {
  amount_cents: { label: "Cost", color: "chart-4" },
} satisfies ChartConfig;

const RUN_VOLUME_CONFIG = {
  run_count: { label: "Runs", color: "chart-1" },
} satisfies ChartConfig;

const SUCCESS_RATE_CONFIG = {
  success_rate: { label: "Success rate", color: "chart-1" },
} satisfies ChartConfig;

const formatDollars = (value: number) => `$${(value / 100).toFixed(2)}`;
const formatDollarsAxis = (value: number) => `$${(value / 100).toFixed(0)}`;
const formatPercent = (value: number) => `${value.toFixed(1)}%`;

const analyticsSearchSchema = z.object({
  window: z.enum(["7d", "30d", "90d"]).optional().catch(undefined),
});

export const Route = createFileRoute("/app/analytics")({
  validateSearch: zodValidator(analyticsSearchSchema),
  loaderDeps: ({ search }) => ({ window: search.window ?? "30d" }),
  loader: async ({ context, deps }) => {
    await Promise.allSettled([
      context.queryClient.ensureQueryData(costTrendsQueryOptions(deps.window)),
      context.queryClient.ensureQueryData(topCostsQueryOptions(deps.window)),
      context.queryClient.ensureQueryData(performanceQueryOptions(deps.window)),
    ]);
  },
  head: () => ({ meta: seo({ title: "Analytics" }) }),
  errorComponent: ErrorComponent,
  component: AnalyticsPage,
});

function AnalyticsPage() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();
  const selectedWindow = search.window ?? "30d";

  const { data: costTrends } = useQuery(costTrendsQueryOptions(selectedWindow));
  const { data: topCosts } = useQuery(topCostsQueryOptions(selectedWindow));
  const { data: performance } = useQuery(
    performanceQueryOptions(selectedWindow)
  );

  const costData = costTrends ?? [];
  const topCostData = topCosts ?? [];
  const perfData = performance ?? [];

  return (
    <Shell>
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-balance font-normal text-xl tracking-tight">
            Analytics
          </h1>
          <p className="text-muted-foreground text-sm">
            Overview of cost, performance, and orchestration-run volume.
          </p>
        </div>
        <div className="flex items-center gap-1">
          {WINDOWS.map((w) => (
            <Button
              key={w.value}
              onClick={() =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    window: w.value === "30d" ? undefined : w.value,
                  }),
                })
              }
              variant={selectedWindow === w.value ? "default" : "outline"}
            >
              {w.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Cost trends */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Cost trends</CardTitle>
          </CardHeader>
          <CardContent>
            {costData.length > 0 ? (
              <div className="h-[240px]">
                <AreaChart
                  config={COST_TRENDS_CONFIG}
                  containerHeight={240}
                  data={costData}
                  dataKey="date"
                  legend={false}
                  valueFormatter={formatDollars}
                  yAxisProps={{ tickFormatter: formatDollarsAxis }}
                />
              </div>
            ) : (
              <ChartEmptyState message="No cost data available for this time window." />
            )}
          </CardContent>
        </Card>

        {/* Top cost contributors */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">
              Top cost contributors
            </CardTitle>
          </CardHeader>
          <CardContent>
            {topCostData.length > 0 ? (
              <div className="h-[240px]">
                <BarChart
                  config={TOP_COSTS_CONFIG}
                  containerHeight={240}
                  data={topCostData}
                  dataKey="name"
                  layout="vertical"
                  legend={false}
                  valueFormatter={formatDollars}
                  xAxisProps={{ tickFormatter: formatDollarsAxis }}
                  yAxisProps={{ width: 120 }}
                />
              </div>
            ) : (
              <ChartEmptyState message="No project cost data available for this time window." />
            )}
          </CardContent>
        </Card>

        {/* Run volume */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Run volume</CardTitle>
          </CardHeader>
          <CardContent>
            {costData.length > 0 ? (
              <div className="h-[240px]">
                <BarChart
                  config={RUN_VOLUME_CONFIG}
                  containerHeight={240}
                  data={costData}
                  dataKey="date"
                  legend={false}
                />
              </div>
            ) : (
              <ChartEmptyState message="No run volume data available for this time window." />
            )}
          </CardContent>
        </Card>

        {/* Performance */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-medium text-sm">Success rate</CardTitle>
          </CardHeader>
          <CardContent>
            {perfData.length > 0 ? (
              <div className="h-[240px]">
                <LineChart
                  config={SUCCESS_RATE_CONFIG}
                  containerHeight={240}
                  data={perfData}
                  dataKey="date"
                  legend={false}
                  valueFormatter={formatPercent}
                  yAxisProps={{
                    domain: [0, 100],
                    tickFormatter: (value: number) => `${value}%`,
                  }}
                />
              </div>
            ) : (
              <ChartEmptyState message="No performance data available for this time window." />
            )}
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
