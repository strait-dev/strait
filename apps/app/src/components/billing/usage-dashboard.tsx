import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { capitalize, formatMicroUsd } from "@/lib/format";
import { CHART_COLORS } from "@/lib/status-colors";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import { ChartTooltip } from "../dashboard/chart-tooltip";
import { RadialUsageGauge } from "./radial-usage-gauge";

const RUNS_LABEL_MAP = {
  runs_count: {
    label: "Runs",
    color: CHART_COLORS.active,
  },
};

const COST_DONUT_LABEL_MAP = {
  value: {
    label: "Cost",
    color: CHART_COLORS.active,
    format: formatMicroUsd,
  },
};

const PROJECT_RUNS_LABEL_MAP = {
  runs: {
    label: "Runs",
    color: CHART_COLORS.active,
  },
};

export function UsageDashboard() {
  const {
    data: usage,
    isLoading,
    isError: isUsageError,
  } = useQuery(orgUsageQueryOptions());
  const { data: history } = useQuery(usageHistoryQueryOptions());
  const { data: projectCosts } = useQuery(projectCostsQueryOptions());
  const navigate = useNavigate();

  const handleManageBilling = async () => {
    const result = await getCustomerPortalUrlServerFn();
    if (result.url) {
      window.location.href = result.url;
    }
  };

  const handleViewInvoices = async () => {
    const result = await getCustomerPortalUrlServerFn();
    if (result.url) {
      window.location.href = result.url;
    }
  };

  if (isUsageError) {
    return (
      <div className="flex h-48 items-center justify-center">
        <p className="text-destructive text-sm">
          Failed to load usage data. Please try again later.
        </p>
      </div>
    );
  }

  if (isLoading || !usage) {
    return (
      <div className="flex h-48 items-center justify-center">
        <p className="text-muted-foreground text-sm">Loading usage data...</p>
      </div>
    );
  }

  const planName = capitalize(usage.plan);

  // Aggregate costs from history for donut chart
  const totalCompute = (history ?? []).reduce(
    (sum, d) => sum + d.compute_cost_microusd,
    0
  );
  const totalAi = (history ?? []).reduce(
    (sum, d) => sum + d.ai_cost_microusd,
    0
  );
  const totalCost = totalCompute + totalAi;
  const donutData = [
    { name: "Compute", value: totalCompute, fill: CHART_COLORS.active },
    { name: "AI Cost", value: totalAi, fill: CHART_COLORS.warning },
  ];

  // Top 5 projects by runs
  const topProjects = [...(projectCosts ?? [])]
    .sort((a, b) => b.runs - a.runs)
    .slice(0, 5);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-normal text-foreground text-lg tracking-tight">
            Usage & Billing
          </h2>
          <p className="text-muted-foreground text-sm">
            Current plan: <Badge variant="default">{planName}</Badge>
            {usage.period.start && (
              <span className="ml-2">
                Billing period: {usage.period.start} - {usage.period.end}
              </span>
            )}
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            onClick={() => navigate({ to: "/app/upgrade" })}
            size="sm"
            variant="outline"
          >
            Upgrade Plan
          </Button>
          <Button onClick={handleManageBilling} size="sm" variant="outline">
            Manage Billing
          </Button>
        </div>
      </div>

      {/* Radial Gauges */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <RadialUsageGauge
          display={usage.usage.runs_today.display}
          label="Runs Today"
          limit={usage.usage.runs_today.limit}
          percent={usage.usage.runs_today.percent}
          used={usage.usage.runs_today.used}
        />
        <RadialUsageGauge
          label="Concurrent Runs"
          limit={usage.usage.concurrent_runs.limit}
          percent={usage.usage.concurrent_runs.percent}
          used={usage.usage.concurrent_runs.used}
        />
        <RadialUsageGauge
          display={usage.usage.compute_credit.display}
          label="Compute Credit"
          limit={usage.usage.compute_credit.limit}
          percent={usage.usage.compute_credit.percent}
          used={usage.usage.compute_credit.used}
        />
        <RadialUsageGauge
          label="AI Model Calls"
          limit={usage.usage.ai_model_calls_today.limit}
          percent={usage.usage.ai_model_calls_today.percent}
          used={usage.usage.ai_model_calls_today.used}
        />
      </div>

      {/* Daily Runs Line Chart */}
      {history && history.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="font-medium text-sm">
              Daily Runs (30 days)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="h-[200px]">
              <ResponsiveContainer
                height="100%"
                minHeight={1}
                minWidth={1}
                width="100%"
              >
                <LineChart data={history}>
                  <CartesianGrid
                    className="stroke-border"
                    strokeDasharray="3 3"
                  />
                  <XAxis
                    className="text-muted-foreground"
                    dataKey="date"
                    tick={{ fontSize: 12 }}
                    tickFormatter={(v: string) => v.slice(5)}
                  />
                  <YAxis
                    className="text-muted-foreground"
                    tick={{ fontSize: 12 }}
                  />
                  <Tooltip
                    content={<ChartTooltip labelMap={RUNS_LABEL_MAP} />}
                    cursor={{ stroke: "var(--muted-foreground)" }}
                  />
                  <Line
                    dataKey="runs_count"
                    dot={false}
                    isAnimationActive={false}
                    stroke={CHART_COLORS.active}
                    strokeWidth={2}
                    type="monotone"
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Cost Donut + Top Projects side by side */}
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {/* Cost Breakdown Donut */}
        {totalCost > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-sm">
                Cost Breakdown
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="relative h-[200px]">
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <PieChart>
                    <Pie
                      cx="50%"
                      cy="50%"
                      data={donutData}
                      dataKey="value"
                      innerRadius="55%"
                      isAnimationActive={false}
                      outerRadius="80%"
                      paddingAngle={2}
                    />
                    <Tooltip
                      content={<ChartTooltip labelMap={COST_DONUT_LABEL_MAP} />}
                    />
                  </PieChart>
                </ResponsiveContainer>
                <div className="absolute inset-0 flex flex-col items-center justify-center">
                  <span className="font-medium text-foreground text-lg tabular-nums">
                    {formatMicroUsd(totalCost)}
                  </span>
                  <span className="text-muted-foreground text-xs">Total</span>
                </div>
              </div>
              <div className="mt-2 flex justify-center gap-4">
                <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
                  <span
                    className="size-2 shrink-0 rounded-full"
                    style={{ backgroundColor: CHART_COLORS.active }}
                  />
                  Compute (
                  {totalCost > 0
                    ? Math.round((totalCompute / totalCost) * 100)
                    : 0}
                  %)
                </div>
                <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
                  <span
                    className="size-2 shrink-0 rounded-full"
                    style={{ backgroundColor: CHART_COLORS.warning }}
                  />
                  AI Cost (
                  {totalCost > 0 ? Math.round((totalAi / totalCost) * 100) : 0}
                  %)
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Top Projects by Runs */}
        {topProjects.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-sm">
                Top Projects (runs)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div
                className="h-[200px]"
                style={{
                  height: `${Math.max(200, topProjects.length * 36)}px`,
                }}
              >
                <ResponsiveContainer
                  height="100%"
                  minHeight={1}
                  minWidth={1}
                  width="100%"
                >
                  <BarChart data={topProjects} layout="vertical">
                    <CartesianGrid
                      className="stroke-border"
                      strokeDasharray="3 3"
                    />
                    <XAxis
                      className="text-muted-foreground"
                      tick={{ fontSize: 12 }}
                      type="number"
                    />
                    <YAxis
                      className="text-muted-foreground"
                      dataKey="name"
                      tick={{ fontSize: 12 }}
                      type="category"
                      width={100}
                    />
                    <Tooltip
                      content={
                        <ChartTooltip labelMap={PROJECT_RUNS_LABEL_MAP} />
                      }
                      cursor={{ fill: "var(--muted)" }}
                    />
                    <Bar
                      dataKey="runs"
                      fill={CHART_COLORS.active}
                      isAnimationActive={false}
                      radius={[0, 4, 4, 0]}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Resources */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Resources</CardTitle>
          <CardDescription>Organization resource allocation</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <p className="text-muted-foreground text-xs">Projects</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.projects.used} /{" "}
                {usage.usage.projects.limit === -1
                  ? "Unlimited"
                  : usage.usage.projects.limit}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Members</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.members.used} /{" "}
                {usage.usage.members.limit === -1
                  ? "Unlimited"
                  : usage.usage.members.limit}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Regions</p>
              <p className="font-medium text-foreground tabular-nums">
                {usage.usage.regions_available}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Alerts */}
      {usage.alerts.length > 0 && (
        <Card className="border-yellow-200 dark:border-yellow-800">
          <CardHeader>
            <CardTitle className="text-sm">Alerts</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {usage.alerts.map((alert) => (
                <div
                  className="flex items-center justify-between rounded-custom bg-yellow-50 p-2 dark:bg-yellow-950"
                  key={alert.dimension}
                >
                  <span className="text-sm text-yellow-800 dark:text-yellow-200">
                    {alert.message}
                  </span>
                  <Button
                    onClick={() => navigate({ to: "/app/upgrade" })}
                    size="sm"
                    variant="outline"
                  >
                    Upgrade
                  </Button>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="flex items-center justify-between">
        <p className="text-muted-foreground text-xs">
          Retention: {usage.usage.retention_days} day
          {usage.usage.retention_days === 1 ? "" : "s"}
        </p>
        <div className="flex gap-2">
          <Button onClick={handleManageBilling} size="sm" variant="link">
            Manage Billing in Polar
          </Button>
          <Button onClick={handleViewInvoices} size="sm" variant="link">
            View Invoices
          </Button>
        </div>
      </div>
    </div>
  );
}
