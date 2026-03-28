import { HugeiconsIcon } from "@hugeicons/react";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { createFileRoute } from "@tanstack/react-router";
import { Suspense } from "react";
import AlertsForecastTab from "@/components/billing/alerts-forecast-tab";
import ProjectCostsTab from "@/components/billing/project-costs-tab";
import ReferralProgram from "@/components/billing/referral-program";
import SpendingLimitsTab from "@/components/billing/spending-limits-tab";
import UsageDashboard from "@/components/billing/usage-dashboard";
import UsageHistoryTab from "@/components/billing/usage-history-tab";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import InlineError from "@/components/common/inline-error";
import NotFound from "@/components/common/not-found";
import { QueryErrorBoundary } from "@/components/common/query-error-boundary";
import TabSkeleton from "@/components/common/tab-skeleton";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import { anomalyAlertsQueryOptions } from "@/hooks/billing/use-anomaly-alerts";
import { anomalyConfigQueryOptions } from "@/hooks/billing/use-anomaly-config";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import { referralsQueryOptions } from "@/hooks/billing/use-referrals";
import { spendingLimitQueryOptions } from "@/hooks/billing/use-spending-limit";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";
import { usageHistoryQueryOptions } from "@/hooks/billing/use-usage-history";
import {
  ActivityIcon,
  AlertIcon,
  BriefcaseIcon,
  CreditCardIcon,
  TrendingUpIcon,
  UsersIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/billing/")({
  loader: async ({ context }) => {
    const ctx = context as AppRouteContext;
    await Promise.allSettled([
      ctx.queryClient.ensureQueryData(orgUsageQueryOptions()),
      ctx.queryClient.ensureQueryData(usageHistoryQueryOptions()),
      ctx.queryClient.ensureQueryData(projectCostsQueryOptions()),
      ctx.queryClient.ensureQueryData(spendingLimitQueryOptions()),
      ctx.queryClient.ensureQueryData(anomalyAlertsQueryOptions()),
      ctx.queryClient.ensureQueryData(usageForecastQueryOptions()),
      ctx.queryClient.ensureQueryData(referralsQueryOptions()),
      ctx.queryClient.ensureQueryData(anomalyConfigQueryOptions()),
    ]);
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  usePageEvent("billing_viewed");

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <div>
          <h1 className="font-normal text-foreground text-lg tracking-tight">
            Billing
          </h1>
          <p className="text-muted-foreground text-sm">
            Monitor usage, costs, and spending across your organization.
          </p>
        </div>

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
            <TabsTrigger className="flex items-center gap-2" value="alerts">
              <HugeiconsIcon className="size-4" icon={AlertIcon} />
              Alerts
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="referrals">
              <HugeiconsIcon className="size-4" icon={UsersIcon} />
              Referrals
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
              <Suspense fallback={<TabSkeleton />}>
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
              <Suspense fallback={<TabSkeleton />}>
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
              <Suspense fallback={<TabSkeleton />}>
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
              <Suspense fallback={<TabSkeleton />}>
                <SpendingLimitsTab />
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
              <Suspense fallback={<TabSkeleton />}>
                <AlertsForecastTab />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="referrals">
            <QueryErrorBoundary
              fallback={({ resetErrorBoundary }) => (
                <InlineError
                  message="Failed to load referrals"
                  onRetry={resetErrorBoundary}
                />
              )}
            >
              <Suspense fallback={<TabSkeleton />}>
                <ReferralProgram />
              </Suspense>
            </QueryErrorBoundary>
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
