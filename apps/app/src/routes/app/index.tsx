import { Shell } from "@strait/ui/components/shell";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";

import { GettingStarted } from "@/components/common/getting-started";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription";
import type { AppRouteContext } from "@/routes/app/layout";

const subscriptionSearchSchema = z.object({
  subscription: z.string().optional(),
  t: z.string().optional(),
  checkout_id: z.string().optional(),
  checkout_success: z.coerce.string().optional(),
  quickstart: z.coerce.boolean().optional(),
});

export const Route = createFileRoute("/app/")({
  validateSearch: zodValidator(subscriptionSearchSchema),
  loaderDeps: ({ search }) => ({ quickstart: search.quickstart ?? false }),
  loader: async ({ context, location, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;

    await context.queryClient.ensureQueryData(subscriptionQueryOptions());

    if (hasProject && !deps.quickstart) {
      throw redirect({
        to: "/app/dashboard",
        search: location.search as Record<string, unknown>,
      });
    }

    return { session };
  },
  component: RouteComponent,
});

function RouteComponent() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();
  const { session } = Route.useLoaderData();

  const handleUrlCleanup = useCallback(() => {
    navigate({
      search: {},
      replace: true,
    });
  }, [navigate]);

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
