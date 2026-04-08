import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import {
  notifyDeliveriesQueryOptions,
  notifySubscribersQueryOptions,
} from "@/hooks/api/use-notify";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(notifyDeliveriesQueryOptions()),
        context.queryClient.ensureQueryData(notifySubscribersQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  errorComponent: ErrorComponent,
  component: NotifyOverviewPage,
});

function NotifyOverviewPage() {
  const { hasProject, session } = Route.useLoaderData();

  const deliveriesQuery = useQuery({
    ...notifyDeliveriesQueryOptions(),
    enabled: hasProject,
  });
  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions(),
    enabled: hasProject,
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const deliveries = deliveriesQuery.data ?? [];
  const subscribers = subscribersQuery.data ?? [];

  const deliveredCount = deliveries.filter(
    (item) => item.status === "delivered"
  ).length;
  const failedCount = deliveries.filter(
    (item) => item.status === "failed" || item.status === "bounced"
  ).length;

  return (
    <Shell>
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Subscribers</CardTitle>
            <CardDescription>Total subscribers in this project</CardDescription>
          </CardHeader>
          <CardContent className="text-2xl">{subscribers.length}</CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Delivered</CardTitle>
            <CardDescription>
              Delivered notifications in current list window
            </CardDescription>
          </CardHeader>
          <CardContent className="text-2xl">{deliveredCount}</CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Failed/Bounced</CardTitle>
            <CardDescription>
              Needs attention and possible suppression review
            </CardDescription>
          </CardHeader>
          <CardContent className="text-2xl">{failedCount}</CardContent>
        </Card>
      </div>

      <div className="mt-6 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Deliveries</CardTitle>
            <CardDescription>
              Track notify message lifecycle and outcomes.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/deliveries"
            >
              Open deliveries
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Subscribers</CardTitle>
            <CardDescription>
              Manage recipients, profiles, and suppression controls.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/subscribers"
            >
              Open subscribers
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Templates</CardTitle>
            <CardDescription>
              Create, update, and preview channel templates.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/templates"
            >
              Open templates
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Topics</CardTitle>
            <CardDescription>
              Group subscribers and manage topic memberships.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/topics"
            >
              Open topics
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Categories</CardTitle>
            <CardDescription>
              Define policy semantics for notification groups.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/categories"
            >
              Open categories
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Providers</CardTitle>
            <CardDescription>
              Configure SES provider settings and delivery defaults.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/providers"
            >
              Open providers
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Policies</CardTitle>
            <CardDescription>
              Manage scope-based digest, retry, and escalation overrides.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/policies"
            >
              Open policies
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Compose</CardTitle>
            <CardDescription>
              Preview, test send, and trigger notify events.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              className="text-primary text-sm underline"
              to="/app/notify/compose"
            >
              Open compose
            </Link>
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
