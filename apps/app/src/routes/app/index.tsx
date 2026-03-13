import { Shell } from "@strait/ui/components/shell.tsx";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback } from "react";
import * as z from "zod";
import DashboardPage from "@/components/common/dashboard-page.tsx";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog.tsx";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription.ts";
import type { Session } from "@/routes/__root.tsx";

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
  const { session } = Route.useLoaderData();
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
      <DashboardPage session={session} />
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
