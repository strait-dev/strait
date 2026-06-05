import { HugeiconsIcon } from "@hugeicons/react";
import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { lazy, Suspense } from "react";
import { EnterpriseOverview } from "@/components/billing/enterprise-overview";
import SpendingLimitSetupBanner from "@/components/billing/spending-limit-setup-banner";
import UsageDashboard from "@/components/billing/usage-dashboard";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import InlineError from "@/components/common/inline-error";
import NotFound from "@/components/common/not-found";
import { QueryErrorBoundary } from "@/components/common/query-error-boundary";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import { anomalyAlertsQueryOptions } from "@/hooks/billing/use-anomaly-alerts";
import { anomalyConfigQueryOptions } from "@/hooks/billing/use-anomaly-config";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { spendingLimitQueryOptions } from "@/hooks/billing/use-spending-limit";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import { isCommunityEdition } from "@/lib/edition";
import {
  ActivityIcon,
  AlertIcon,
  BriefcaseIcon,
  CreditCardIcon,
  TrendingUpIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

const AddonsTab = lazy(() => import("@/components/billing/addons-tab"));
const AlertsForecastTab = lazy(
  () => import("@/components/billing/alerts-forecast-tab")
);
const ProjectCostsTab = lazy(
  () => import("@/components/billing/project-costs-tab")
);
const SpendingLimitsTab = lazy(
  () => import("@/components/billing/spending-limits-tab")
);
const UsageHistoryTab = lazy(
  () => import("@/components/billing/usage-history-tab")
);

const tabFallback = <Skeleton className="h-64" />;

export const Route = createFileRoute("/app/billing/")({
  head: () => ({ meta: [{ title: "Billing · Strait" }] }),
  // Cloud-only: Billing, usage history, spending limits, and Stripe
  // addon purchases are not available in the community edition.
  // See `src/lib/edition.ts` for the gate.
  beforeLoad: () => {
    if (isCommunityEdition) {
      throw redirect({ to: "/app" });
    }
  },
  loader: async ({ context }) => {
    const ctx = context as AppRouteContext;
    await Promise.allSettled([
      ctx.queryClient.ensureQueryData(orgUsageQueryOptions()),
      ctx.queryClient.ensureQueryData(usageHistoryQueryOptions()),
      ctx.queryClient.ensureQueryData(projectCostsQueryOptions()),
      ctx.queryClient.ensureQueryData(spendingLimitQueryOptions()),
      ctx.queryClient.ensureQueryData(anomalyAlertsQueryOptions()),
      ctx.queryClient.ensureQueryData(usageForecastQueryOptions()),
      ctx.queryClient.ensureQueryData(anomalyConfigQueryOptions()),
    ]);
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  usePageEvent("billing_viewed");
  const { data: orgUsage } = useQuery(orgUsageQueryOptions());
  const isEnterprise = orgUsage?.plan === "enterprise";

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <div>
          <h1 className="text-balance font-normal text-xl tracking-tight">
            Billing
          </h1>
          <p className="text-muted-foreground text-sm">
            Monitor usage, costs, and spending across your organization.
          </p>
        </div>

        {isEnterprise && orgUsage?.enterprise_tier ? (
          <EnterpriseOverview
            contractEndDate={orgUsage.contract_end_date ?? ""}
            enterpriseTier={orgUsage.enterprise_tier}
            overageDiscountPct={orgUsage.overage_discount_pct ?? 0}
            periodSpendMicro={orgUsage.period_spend_microusd}
            slaUptimePct={orgUsage.sla_uptime_pct ?? 0}
          />
        ) : null}

        <SpendingLimitSetupBanner />

        <Tabs className="w-full" defaultValue="overview">
          <TabsList>
            <TabsTrigger className="flex items-center gap-2" value="overview">
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Overview
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="history">
              <HugeiconsIcon className="size-4" icon={ActivityIcon} />
              Usage History
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="projects">
              <HugeiconsIcon className="size-4" icon={BriefcaseIcon} />
              Project Costs
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="spending">
              <HugeiconsIcon className="size-4" icon={TrendingUpIcon} />
              Spending
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="addons">
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Add-ons
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="alerts">
              <HugeiconsIcon className="size-4" icon={AlertIcon} />
              Alerts
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="overview">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load usage overview"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <UsageDashboard />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="history">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load usage history"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <UsageHistoryTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="projects">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load project costs"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <ProjectCostsTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="spending">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load spending limits"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <SpendingLimitsTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="addons">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load add-ons"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <AddonsTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="alerts">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load alerts"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={tabFallback}>
                <AlertsForecastTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
