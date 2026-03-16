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
import ApiKeysManagement from "@/components/(settings)/api-keys-management";
import OrganizationInfo from "@/components/(settings)/organization-info";
import PendingInvitations from "@/components/(settings)/pending-invitations";
import SubscriptionOverview from "@/components/(settings)/subscription-overview";
import TeamMembers from "@/components/(settings)/team-members";
import { DefaultCatchBoundary } from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import PageHeader from "@/components/common/page-header";
import { BuildingIcon, CreditCardIcon, KeyIcon, UsersIcon } from "@/lib/icons";
import type { Session } from "@/routes/__root";

type LoaderData = {
  session: NonNullable<Session>;
};

export const Route = createFileRoute("/app/org/$id")({
  loader: ({ context }) => {
    const session = context.session as unknown as Session;
    if (!session) {
      throw new Error("Session unexpectedly null");
    }
    return { session } as LoaderData;
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  const { session } = Route.useLoaderData() as LoaderData;
  const params = Route.useParams();
  const organizationId = params.id;

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <PageHeader
          text="Manage your organization settings, billing, and team."
          title="Organization Settings"
        />

        <Tabs className="w-full" defaultValue="organization">
          <TabsList>
            <TabsTrigger
              className="flex items-center gap-2"
              value="organization"
            >
              <HugeiconsIcon className="size-4" icon={BuildingIcon} />
              Organization
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              value="subscription"
            >
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Subscription
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="api-keys">
              <HugeiconsIcon className="size-4" icon={KeyIcon} />
              API Keys
            </TabsTrigger>
            <TabsTrigger className="flex items-center gap-2" value="team">
              <HugeiconsIcon className="size-4" icon={UsersIcon} />
              Team
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="organization">
            <OrganizationInfo organizationId={organizationId} />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="subscription">
            <Suspense
              fallback={
                <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
                  Loading subscription...
                </div>
              }
            >
              <SubscriptionOverview />
            </Suspense>
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="api-keys">
            <ApiKeysManagement />
          </TabsContent>

          <TabsContent className="mt-6 space-y-6" value="team">
            <PendingInvitations />
            <TeamMembers
              currentUserId={session.user.id}
              organizationId={organizationId}
            />
          </TabsContent>
        </Tabs>
      </div>
    </Shell>
  );
}
