import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { CHART_COLORS, type ChartConfig } from "@strait/ui/components/chart";
import { BarChart, LineChart } from "@strait/ui/components/charts";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
import {
  NoticeBanner,
  NoticeBannerAction,
} from "@strait/ui/components/notice-banner";
import { RadialGauge } from "@strait/ui/components/radial-gauge";
import { Skeleton } from "@strait/ui/components/skeleton";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { capitalize, formatMicroUsd } from "@/lib/format";
import { CheckCircleIcon } from "@/lib/icons";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import OverageWarningBanner from "./overage-warning-banner";

const RUNS_CHART_CONFIG = {
  runs_count: {
    label: "Runs",
    color: "chart-3",
  },
} satisfies ChartConfig;

const PROJECT_RUNS_CHART_CONFIG = {
  runs: {
    label: "Runs",
    color: "chart-3",
  },
} satisfies ChartConfig;

type UsageGaugeData = {
  label: string;
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

function getGaugeColor(percent: number): string {
  if (percent >= 90) {
    return CHART_COLORS["chart-2"];
  }
  if (percent >= 70) {
    return CHART_COLORS["chart-4"];
  }
  return CHART_COLORS["chart-3"];
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
            <Badge iconLeft={CheckCircleIcon} size="lg" variant="success-light">
              Unlimited
            </Badge>
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
      <NoticeBanner title="Failed to load usage data" variant="destructive">
        Please try again later.
      </NoticeBanner>
    );
  }

  if (isLoading || !usage) {
    return <Skeleton className="h-48 w-full" />;
  }

  const planName = capitalize(usage.plan);

  const totalRunCost = (history ?? []).reduce(
    (sum, d) => sum + d.spend_microusd,
    0
  );
  const totalCost = totalRunCost;

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
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
        {renderUsageGauge({
          display: usage.usage.monthly_runs.display,
          label: "Runs This Month",
          limit: usage.usage.monthly_runs.limit,
          percent: usage.usage.monthly_runs.percent,
          used: usage.usage.monthly_runs.used,
        })}
        {renderUsageGauge({
          label: "Concurrent Runs",
          limit: usage.usage.concurrent_runs.limit,
          percent: usage.usage.concurrent_runs.percent,
          used: usage.usage.concurrent_runs.used,
        })}
        <Card>
          <CardContent className="flex h-full min-h-[152px] flex-col justify-center p-4">
            <p className="text-muted-foreground text-xs">Period Spend</p>
            <p className="mt-3 font-medium text-2xl tabular-nums">
              {formatMicroUsd(usage.period_spend_microusd)}
            </p>
            <p className="mt-1 text-muted-foreground text-xs">
              Current billing period
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Transitional message when in-flight runs exceed new plan limit */}
      {usage.usage.concurrent_runs.limit > 0 &&
        usage.usage.concurrent_runs.used >
          usage.usage.concurrent_runs.limit && (
          <NoticeBanner title="Concurrent run limit exceeded" variant="warning">
            {usage.usage.concurrent_runs.used -
              usage.usage.concurrent_runs.limit}{" "}
            runs are finishing from the previous plan. New runs will start once
            slots free up.
          </NoticeBanner>
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

      {/* Runs by day line chart */}
      {history && history.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="font-medium text-sm">
              Runs by Day (30 days)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="h-[200px]">
              <LineChart
                config={RUNS_CHART_CONFIG}
                containerHeight={200}
                data={history}
                dataKey="date"
                legend={false}
                xAxisProps={{
                  tickFormatter: (value: string) => value.slice(5),
                }}
              />
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-3">
        {topProjects.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-sm">
                Top Projects (runs)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <BarChart
                config={PROJECT_RUNS_CHART_CONFIG}
                containerHeight={Math.max(200, topProjects.length * 36)}
                data={topProjects}
                dataKey="name"
                layout="vertical"
                legend={false}
                yAxisProps={{ width: 100 }}
              />
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
          <DescriptionList divided orientation="horizontal" size="sm">
            <DescriptionTerm>Projects</DescriptionTerm>
            <DescriptionDetails className="text-right tabular-nums">
              {usage.usage.projects.used} /{" "}
              {usage.usage.projects.limit === -1
                ? "Unlimited"
                : usage.usage.projects.limit}
            </DescriptionDetails>
            <DescriptionTerm>Members</DescriptionTerm>
            <DescriptionDetails className="text-right tabular-nums">
              {usage.usage.members.used} /{" "}
              {usage.usage.members.limit === -1
                ? "Unlimited"
                : usage.usage.members.limit}
            </DescriptionDetails>
          </DescriptionList>
        </CardContent>
      </Card>

      {/* Overage Warning */}
      {usage.overage_microusd > 0 && usage.plan !== "free" && (
        <NoticeBanner
          action={
            <NoticeBannerAction>
              <Button onClick={() => navigate({ to: "/app/billing" })}>
                Set limit
              </Button>
            </NoticeBannerAction>
          }
          title="You are in overage"
          variant="warning"
        >
          ${(usage.overage_microusd / 1_000_000).toFixed(2)} beyond your
          included run allowance. Set a spending cap to control costs.
        </NoticeBanner>
      )}

      {/* Alerts */}
      {usage.alerts.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Alerts</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {usage.alerts.map((alert) => (
                <NoticeBanner
                  action={
                    <NoticeBannerAction>
                      <Button onClick={() => navigate({ to: "/app/upgrade" })}>
                        Upgrade
                      </Button>
                    </NoticeBannerAction>
                  }
                  key={alert.dimension}
                  size="sm"
                  variant="warning"
                >
                  {alert.message}
                </NoticeBanner>
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
