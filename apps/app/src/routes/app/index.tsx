import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import * as z from "zod";
import { GettingStarted } from "@/components/common/getting-started";
import SubscriptionSuccessDialog from "@/components/subscription/subscription-success-dialog";
import { subscriptionQueryOptions } from "@/hooks/subscription/use-subscription";
import { seo } from "@/lib/seo";
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

    await context.queryClient.ensureQueryData(subscriptionQueryOptions());

    return { session };
  },
  head: () => ({ meta: seo({ title: "Getting started" }) }),
  component: RouteComponent,
});

function RouteComponent() {
  const navigate = Route.useNavigate();
  const search = Route.useSearch();
  const { session } = Route.useLoaderData();

  const handleUrlCleanup = () => {
    navigate({
      search: {},
      replace: true,
    });
  };

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
