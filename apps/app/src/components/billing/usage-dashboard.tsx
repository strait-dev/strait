import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { RadialGauge } from "@strait/ui/components/radial-gauge";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  Pie,
  PieChart,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { capitalize, formatMicroUsd } from "@/lib/format";
import { CheckCircleIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import ChartTooltip from "../dashboard/chart-tooltip";
import ResponsiveChartContainer from "../dashboard/responsive-chart-container";
import OverageWarningBanner from "./overage-warning-banner";

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

type UsageGaugeData = {
  label: string;
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

function getGaugeColor(percent: number): string {
  if (percent >= 90) {
    return CHART_COLORS.error;
  }
  if (percent >= 70) {
    return CHART_COLORS.warning;
  }
  return CHART_COLORS.active;
}

function renderUsageGauge({
  label,
  used,
  limit,
  percent,
  display,
}: UsageGaugeData) {
  const isUnlimited = limit === -1;
  const displayValue = display || `${used.toLocaleString()}`;
  const limitDisplay = isUnlimited ? "Unlimited" : limit.toLocaleString();

  return (
    <Card key={label}>
      <CardContent className="p-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        {isUnlimited ? (
          <div className="flex h-[120px] items-center justify-center">
            <HugeiconsIcon
              className="text-success"
              icon={CheckCircleIcon}
              size={32}
            />
          </div>
        ) : (
          <RadialGauge
            centerLabel={displayValue}
            className="h-[120px]"
            color={getGaugeColor(percent)}
            label={`/ ${limitDisplay}`}
            value={percent}
          />
        )}
      </CardContent>
    </Card>
  );
}

const UsageDashboard = () => {
  const {
    data: usage,
    isLoading,
    isError: isUsageError,
  } = useQuery(orgUsageQueryOptions());
  const { data: history } = useQuery(usageHistoryQueryOptions());
  const { data: projectCosts } = useQuery(projectCostsQueryOptions());
  const navigate = useNavigate();
  const [isRedirecting, setIsRedirecting] = useState(false);

  const handleManageBilling = async () => {
    setIsRedirecting(true);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.url) {
        window.location.href = result.url;
      }
    } catch {
      toast.error("Failed to open billing portal");
    } finally {
      setIsRedirecting(false);
    }
  };

  const handleViewInvoices = async () => {
    setIsRedirecting(true);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.url) {
        window.location.href = result.url;
      }
    } catch {
      toast.error("Failed to load invoices");
    } finally {
      setIsRedirecting(false);
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
      {/* Overage warning */}
      <OverageWarningBanner />

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-balance font-normal text-foreground text-lg tracking-tight">
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
            variant="outline"
          >
            Upgrade Plan
          </Button>
          <Button
            disabled={isRedirecting}
            onClick={handleManageBilling}
            variant="outline"
          >
            Manage Billing
          </Button>
        </div>
      </div>

      {/* Radial Gauges */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {renderUsageGauge({
          display: usage.usage.runs_today.display,
          label: "Runs Today",
          limit: usage.usage.runs_today.limit,
          percent: usage.usage.runs_today.percent,
          used: usage.usage.runs_today.used,
        })}
        {renderUsageGauge({
          label: "Concurrent Runs",
          limit: usage.usage.concurrent_runs.limit,
          percent: usage.usage.concurrent_runs.percent,
          used: usage.usage.concurrent_runs.used,
        })}
        {renderUsageGauge({
          display: usage.usage.compute_credit.display,
          label: "Compute Credit",
          limit: usage.usage.compute_credit.limit,
          percent: usage.usage.compute_credit.percent,
          used: usage.usage.compute_credit.used,
        })}
        {renderUsageGauge({
          label: "AI Model Calls",
          limit: usage.usage.ai_model_calls_today.limit,
          percent: usage.usage.ai_model_calls_today.percent,
          used: usage.usage.ai_model_calls_today.used,
        })}
      </div>

      {/* Transitional message when in-flight runs exceed new plan limit */}
      {usage.usage.concurrent_runs.limit > 0 &&
        usage.usage.concurrent_runs.used >
          usage.usage.concurrent_runs.limit && (
          <div className="rounded-lg border border-border bg-muted/50 px-4 py-2">
            <p className="text-muted-foreground text-sm">
              {usage.usage.concurrent_runs.used -
                usage.usage.concurrent_runs.limit}{" "}
              runs finishing from previous plan. New runs will start once slots
              free up.
            </p>
          </div>
        )}

      {/* Estimated bill card */}
      {totalCost > 0 && (
        <Card>
          <CardContent className="flex items-center justify-between p-4">
            <div>
              <p className="text-muted-foreground text-xs">
                Estimated Bill This Month
              </p>
              <p className="font-medium text-foreground text-lg tabular-nums">
                {formatMicroUsd(totalCost)}
              </p>
            </div>
            <p className="text-muted-foreground text-xs">
              Based on usage through{" "}
              {new Date().toLocaleDateString("en-US", {
                month: "short",
                day: "numeric",
              })}
            </p>
          </CardContent>
        </Card>
      )}

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
              <ResponsiveChartContainer
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
              </ResponsiveChartContainer>
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
                <ResponsiveChartContainer
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
                </ResponsiveChartContainer>
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
                <ResponsiveChartContainer
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
                </ResponsiveChartContainer>
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

      {/* Overage Warning */}
      {usage.overage_microusd > 0 && usage.plan !== "free" && (
        <Card className="border-warning/30">
          <CardContent className="flex items-center justify-between py-3">
            <div className="flex flex-col gap-0.5">
              <span className="font-medium text-sm text-warning">
                You are in overage
              </span>
              <span className="text-muted-foreground text-xs">
                ${(usage.overage_microusd / 1_000_000).toFixed(2)} over your
                included $
                {(usage.included_credit_microusd / 1_000_000).toFixed(2)}{" "}
                credit. Set a spending limit to control costs.
              </span>
            </div>
            <Button
              onClick={() => navigate({ to: "/app/billing" })}
              variant="outline"
            >
              Set Limit
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Alerts */}
      {usage.alerts.length > 0 && (
        <Card className="border-warning/30">
          <CardHeader>
            <CardTitle className="text-sm">Alerts</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {usage.alerts.map((alert) => (
                <div
                  className="flex items-center justify-between rounded bg-warning/5 p-2"
                  key={alert.dimension}
                >
                  <span className="text-sm text-warning">{alert.message}</span>
                  <Button
                    onClick={() => navigate({ to: "/app/upgrade" })}
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
          <Button
            disabled={isRedirecting}
            onClick={handleManageBilling}
            variant="link"
          >
            Manage Billing
          </Button>
          <Button
            disabled={isRedirecting}
            onClick={handleViewInvoices}
            variant="link"
          >
            View Invoices
          </Button>
        </div>
      </div>
    </div>
  );
};

export default UsageDashboard;
