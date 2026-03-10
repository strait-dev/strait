import { CreditCardIcon, UserIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { createFileRoute } from "@tanstack/react-router";
import Account from "@/components/(settings)/account";
import SubscriptionOverview from "@/components/(settings)/subscription-overview";
import { DefaultCatchBoundary } from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import PageHeader from "@/components/common/page-header";
import type { Session } from "@/routes/__root";

export const Route = createFileRoute("/app/settings/")({
  loader: ({ context }) => {
    const session = context.session as unknown as Session;
    if (!session) {
      throw new Error("Session unexpectedly null");
    }
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
        <PageHeader
          text="Here you can configure your account and store information."
          title="Settings"
        />

        <Tabs className="w-full" defaultValue="account">
          <TabsList>
            <TabsTrigger className="flex items-center gap-2" value="account">
              <HugeiconsIcon className="size-4" icon={UserIcon} />
              Account
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              value="subscription"
            >
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Subscription
            </TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-6" value="account">
            <Account user={session.user} />
          </TabsContent>

          <TabsContent className="space-y-6" value="subscription">
            <SubscriptionOverview />
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
