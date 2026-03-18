import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";

import { GettingStarted } from "@/components/common/getting-started";
import { MetricsCard } from "@/components/dashboard/metrics-card";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
import {
  analyticsQueryOptions,
  statsQueryOptions,
} from "@/hooks/api/use-dashboard";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription";
import {
  ActivityIcon,
  AlertIcon,
  ArrowRightIcon,
  BriefcaseIcon,
  CalendarIcon,
  CheckCircleIcon,
  ClockIcon,
  WorkflowIcon,
  ZapIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";
import type { AuthUser, Session } from "@/routes/__root";

const subscriptionSearchSchema = z.object({
  subscription: z.string().optional(),
  t: z.string().optional(),
  checkout_id: z.string().optional(),
  checkout_success: z.coerce.string().optional(),
});

export const Route = createFileRoute("/app/")({
  validateSearch: zodValidator(subscriptionSearchSchema),
  loader: async ({ context }) => {
    const session = context.session as unknown as Session;
    if (!session) {
      throw new Error("Session unexpectedly null");
    }

    const hasProject = !!(session.user as AuthUser).activeProjectId;

    await context.queryClient.ensureQueryData(subscriptionQueryOptions());

    // Only prefetch data queries if user has a project
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(statsQueryOptions()).catch(() => null),
        context.queryClient.ensureQueryData(analyticsQueryOptions(24)).catch(() => null),
      ]);
    }

    return { session, hasProject };
  },
  component: RouteComponent,
});

function RouteComponent() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();
  const { session, hasProject } = Route.useLoaderData() as {
    session: NonNullable<Session>;
    hasProject: boolean;
  };

  const handleUrlCleanup = useCallback(() => {
    navigate({
      search: {},
      replace: true,
    });
  }, [navigate]);

  if (!hasProject) {
    return (
      <Shell>
        <GettingStarted user={session.user} />
        <SubscriptionSuccessDialog
          checkoutId={search.checkout_id}
          isNewSubscription={!!search.checkout_success}
          isUpgrade={!!search.subscription}
          onUrlCleanup={handleUrlCleanup}
          timestamp={search.t}
        />
      </Shell>
    );
  }

  return (
    <Shell>
      <OverviewMetrics />

      <div className="flex justify-center">
        <Button render={<Link to="/app/dashboard" />} variant="outline">
          View dashboard
          <HugeiconsIcon icon={ArrowRightIcon} size={16} />
        </Button>
      </div>

      <SubscriptionSuccessDialog
        checkoutId={search.checkout_id}
        isNewSubscription={!!search.checkout_success}
        isUpgrade={!!search.subscription}
        onUrlCleanup={handleUrlCleanup}
        timestamp={search.t}
      />
    </Shell>
  );
}

function OverviewMetrics() {
  const { data: stats } = useQuery(statsQueryOptions());
  const { data: analytics } = useQuery(analyticsQueryOptions(24));

  const health = analytics?.health_summary;
  const throughput = analytics?.throughput;

  const totalRuns = throughput
    ? throughput.completed + throughput.failed + throughput.timed_out + throughput.canceled
    : 0;
  const successRate = health?.success_rate ?? 0;
  const failedRuns = throughput?.failed ?? 0;
  const avgDuration = health?.avg_duration_secs ?? 0;

  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={ActivityIcon}
          title="Total Runs (24h)"
          value={totalRuns}
        />
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[0]}
          icon={CheckCircleIcon}
          title="Success Rate"
          value={`${successRate.toFixed(1)}%`}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[0]}
          icon={AlertIcon}
          title="Failed Runs"
          value={failedRuns}
        />
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={ClockIcon}
          title="Avg Duration"
          value={`${avgDuration.toFixed(1)}s`}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[0]}
          icon={BriefcaseIcon}
          title="Active Jobs"
          value={health?.active_jobs ?? 0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[0]}
          icon={WorkflowIcon}
          title="Workflows"
          value={health?.total_jobs ?? 0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[0]}
          icon={ZapIcon}
          title="Dead Letter"
          value={0}
        />
        <MetricsCard
          chartColor={CHART_COLORS.neutral}
          chartData={[0]}
          description={`Queue depth: ${stats?.queued ?? 0}`}
          icon={CalendarIcon}
          title="Queued"
          value={stats?.queued ?? 0}
        />
      </div>
    </>
  );
}
