import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { createFileRoute, Link } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";

import { MetricsCard } from "@/components/dashboard/metrics-card";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
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
      {/* Metrics Row 1 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          change={{ value: 12.5, label: "vs yesterday" }}
          chartColor={CHART_COLORS.active}
          chartData={[52, 38, 71, 95, 82, 64, 58]}
          icon={ActivityIcon}
          title="Total Runs (24h)"
          value="3,847"
        />
        <MetricsCard
          change={{ value: 2.1, label: "vs yesterday" }}
          chartColor={CHART_COLORS.success}
          chartData={[95, 96, 97, 98, 97, 98, 97]}
          icon={CheckCircleIcon}
          title="Success Rate"
          value="97.4%"
        />
        <MetricsCard
          change={{ value: -18, label: "vs yesterday" }}
          chartColor={CHART_COLORS.error}
          chartData={[5, 3, 7, 4, 2, 1, 2]}
          icon={AlertIcon}
          title="Failed Runs"
          value="89"
        />
        <MetricsCard
          change={{ value: -8, label: "vs yesterday" }}
          chartColor={CHART_COLORS.active}
          chartData={[4.8, 4.5, 4.3, 4.1, 4.0, 4.2, 4.2]}
          icon={ClockIcon}
          title="Avg Duration"
          value="4.2s"
        />
      </div>

      {/* Metrics Row 2 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricsCard
          chartColor={CHART_COLORS.success}
          chartData={[20, 22, 21, 23, 24, 24, 24]}
          description="Across 3 environments"
          icon={BriefcaseIcon}
          title="Active Jobs"
          value={24}
        />
        <MetricsCard
          chartColor={CHART_COLORS.active}
          chartData={[10, 11, 11, 12, 12, 12, 12]}
          description="8 active, 4 paused"
          icon={WorkflowIcon}
          title="Workflows"
          value={12}
        />
        <MetricsCard
          chartColor={CHART_COLORS.error}
          chartData={[12, 10, 9, 8, 8, 7, 7]}
          description="Awaiting review"
          icon={ZapIcon}
          title="Dead Letter"
          value={7}
        />
        <MetricsCard
          chartColor={CHART_COLORS.neutral}
          chartData={[148, 150, 152, 154, 155, 156, 156]}
          description="Next: 2m 34s"
          icon={CalendarIcon}
          title="Scheduled"
          value={156}
        />
      </div>

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
