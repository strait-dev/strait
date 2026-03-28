import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { createFileRoute, Link } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";

import { GettingStarted } from "@/components/common/getting-started";
import OverviewMetrics from "@/components/dashboard/overview-metrics";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
import {
  analyticsQueryOptions,
  statsQueryOptions,
} from "@/hooks/api/use-dashboard";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription";
import { ArrowRightIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

const subscriptionSearchSchema = z.object({
  subscription: z.string().optional(),
  t: z.string().optional(),
  checkout_id: z.string().optional(),
  checkout_success: z.coerce.string().optional(),
});

export const Route = createFileRoute("/app/")({
  validateSearch: zodValidator(subscriptionSearchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;

    const hasProject = !!session.user.activeProjectId;

    await context.queryClient.ensureQueryData(subscriptionQueryOptions());

    // Only prefetch data queries if user has a project
    if (hasProject) {
      await Promise.all([
        context.queryClient
          .ensureQueryData(statsQueryOptions())
          .catch(() => null),
        context.queryClient
          .ensureQueryData(analyticsQueryOptions(24))
          .catch(() => null),
      ]);
    }

    return { session, hasProject };
  },
  component: RouteComponent,
});

function RouteComponent() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();
  const { session, hasProject } = Route.useLoaderData();

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
      <SubscriptionSuccessDialog
        checkoutId={search.checkout_id}
        isNewSubscription={!!search.checkout_success}
        isUpgrade={!!search.subscription}
        onUrlCleanup={handleUrlCleanup}
        timestamp={search.t}
      />

      <div className="flex justify-center">
        <Button render={<Link to="/app/dashboard" />} variant="outline">
          View dashboard
          <HugeiconsIcon icon={ArrowRightIcon} size={16} />
        </Button>
      </div>
    </Shell>
  );
}
