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
import Account from "@/components/(settings)/account";
import { UsageDashboard } from "@/components/billing/usage-dashboard";
import { DefaultCatchBoundary } from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";

import { CreditCardIcon, UserIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/settings/")({
  loader: ({ context }) => {
    const { session } = context as AppRouteContext;
    return {
      session,
    };
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  const { session } = Route.useLoaderData();

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <Tabs className="w-full" defaultValue="account">
          <TabsList>
            <TabsTrigger className="flex items-center gap-2" value="account">
              <HugeiconsIcon className="size-4" icon={UserIcon} />
              Account
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="billing">
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Usage & Billing
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="account">
            <Account user={session.user} />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="billing">
            <Suspense
              fallback={
                <div className="h-64 animate-pulse rounded-lg bg-muted" />
              }
            >
              <UsageDashboard />
            </Suspense>
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
