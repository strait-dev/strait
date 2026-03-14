import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";
import PageHeader from "@/components/common/page-header";
import { LiveActivityFeed } from "@/components/dashboard/live-activity-feed";
import { MetricsCard } from "@/components/dashboard/metrics-card";
import { RecentRunsTable } from "@/components/dashboard/recent-runs-table";
import { RunsChart } from "@/components/dashboard/runs-chart";
import { StatusDistributionChart } from "@/components/dashboard/status-distribution-chart";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription";
import {
  ActivityIcon,
  AlertIcon,
  BriefcaseIcon,
  CalendarIcon,
  CheckCircleIcon,
  ClockIcon,
  WorkflowIcon,
  ZapIcon,
} from "@/lib/icons";
import type { Session } from "@/routes/__root";

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

    await context.queryClient.ensureQueryData(subscriptionQueryOptions());

    return { session };
  },
  component: RouteComponent,
});

function RouteComponent() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();

  const handleUrlCleanup = useCallback(() => {
    navigate({
      search: {},
      replace: true,
    });
  }, [navigate]);

  return (
    <Shell>
      <PageHeader
        text="Monitor your orchestration infrastructure at a glance."
        title="Dashboard"
      />

      {/* Metrics Row 1 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          change={{ value: 12.5, label: "vs yesterday" }}
          icon={ActivityIcon}
          title="Total Runs (24h)"
          value="3,847"
        />
        <MetricsCard
          change={{ value: 2.1, label: "vs yesterday" }}
          icon={CheckCircleIcon}
          title="Success Rate"
          value="97.4%"
        />
        <MetricsCard
          change={{ value: -18, label: "vs yesterday" }}
          icon={AlertIcon}
          title="Failed Runs"
          value="89"
        />
        <MetricsCard
          change={{ value: -8, label: "vs yesterday" }}
          icon={ClockIcon}
          title="Avg Duration"
          value="4.2s"
        />
      </div>

      {/* Metrics Row 2 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          description="Across 3 environments"
          icon={BriefcaseIcon}
          title="Active Jobs"
          value={24}
        />
        <MetricsCard
          description="8 active, 4 paused"
          icon={WorkflowIcon}
          title="Workflows"
          value={12}
        />
        <MetricsCard
          description="Awaiting review"
          icon={ZapIcon}
          title="Dead Letter"
          value={7}
        />
        <MetricsCard
          description="Next: 2m 34s"
          icon={CalendarIcon}
          title="Scheduled"
          value={156}
        />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RunsChart />
        </div>
        <StatusDistributionChart />
      </div>

      {/* Activity */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RecentRunsTable />
        </div>
        <LiveActivityFeed />
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
