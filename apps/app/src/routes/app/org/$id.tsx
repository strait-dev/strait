import { HugeiconsIcon } from "@hugeicons/react";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Suspense } from "react";
import ApiKeysManagement from "@/components/(settings)/api-keys-management";
import DeleteOrganization from "@/components/(settings)/delete-organization";
import OrganizationInfo from "@/components/(settings)/organization-info";
import PendingInvitations from "@/components/(settings)/pending-invitations";
import SubscriptionOverview from "@/components/(settings)/subscription-overview";
import TeamMembers from "@/components/(settings)/team-members";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";

import { organizationQueryOptions } from "@/hooks/auth/use-organization";
import { useOrganizationRole } from "@/hooks/auth/use-permissions";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { BuildingIcon, CreditCardIcon, KeyIcon, UsersIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/org/$id")({
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await context.queryClient.ensureQueryData(
      organizationQueryOptions(params.id)
    );
    return { session };
  },
  head: () => ({ meta: [{ title: "Organization · Strait" }] }),
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  const { session } = Route.useLoaderData();
  const params = Route.useParams();
  const organizationId = params.id;
  const isHydrated = useIsHydrated();
  const { data: organization } = useQuery(
    organizationQueryOptions(organizationId)
  );
  const { isAdmin } = useOrganizationRole(organizationId, session.user.id);

  return (
    <Shell>
      <h1 className="sr-only">Organization</h1>
      <div className="flex w-full flex-col gap-6">
        <Tabs className="w-full" defaultValue="organization">
          <TabsList>
            <TabsTrigger
              className="flex items-center gap-2"
              disabled={!isHydrated}
              value="organization"
            >
              <HugeiconsIcon className="size-4" icon={BuildingIcon} />
              Organization
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              disabled={!isHydrated}
              value="subscription"
            >
              <HugeiconsIcon className="size-4" icon={CreditCardIcon} />
              Subscription
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              disabled={!isHydrated}
              value="api-keys"
            >
              <HugeiconsIcon className="size-4" icon={KeyIcon} />
              API keys
            </TabsTrigger>
            <TabsTrigger
              className="flex items-center gap-2"
              disabled={!isHydrated}
              value="team"
            >
              <HugeiconsIcon className="size-4" icon={UsersIcon} />
              Team
            </TabsTrigger>
          </TabsList>

          <TabsContent className="mt-6 space-y-6" value="organization">
            <OrganizationInfo
              canEdit={isAdmin}
              organizationId={organizationId}
            />
            {isAdmin && organization && (
              <DeleteOrganization
                organizationId={organizationId}
                organizationName={organization.name}
              />
            )}
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
            <ApiKeysManagement projectId={session.user.activeProjectId} />
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
