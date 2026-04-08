import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import {
  notifyDeliveriesQueryOptions,
  notifyPoliciesQueryOptions,
  notifyProvidersQueryOptions,
  notifySubscribersQueryOptions,
} from "@/hooks/api/use-notify";
import { buildNotifyOpsSnapshot } from "@/lib/notify-ops";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/ops")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(
          notifyDeliveriesQueryOptions({ limit: 200 })
        ),
        context.queryClient.ensureQueryData(
          notifySubscribersQueryOptions({ limit: 200 })
        ),
        context.queryClient.ensureQueryData(
          notifyProvidersQueryOptions("email")
        ),
        context.queryClient.ensureQueryData(notifyPoliciesQueryOptions()),
      ]);
    }

    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyOpsPage,
});

function NotifyOpsPage() {
  const { hasProject, session } = Route.useLoaderData();

  const deliveriesQuery = useQuery({
    ...notifyDeliveriesQueryOptions({ limit: 200 }),
    enabled: hasProject,
  });

  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions({ limit: 200 }),
    enabled: hasProject,
  });

  const providersQuery = useQuery({
    ...notifyProvidersQueryOptions("email"),
    enabled: hasProject,
  });

  const policiesQuery = useQuery({
    ...notifyPoliciesQueryOptions(),
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
  const providers = providersQuery.data ?? [];
  const policies = policiesQuery.data ?? [];

  const snapshot = buildNotifyOpsSnapshot({
    deliveries,
    subscribers,
    providers,
    policies,
  });

  const refreshAll = async () => {
    await Promise.all([
      deliveriesQuery.refetch(),
      subscribersQuery.refetch(),
      providersQuery.refetch(),
      policiesQuery.refetch(),
    ]);
    toast.success("Notify operational data refreshed");
  };

  const triageCommand = `cd apps/strait && go run ./scripts/notify-ses-feedback-check --project-id ${session.user.activeProjectId} --database-url "$DATABASE_URL"`;

  const copyTriageCommand = async () => {
    await navigator.clipboard.writeText(triageCommand);
    toast.success("Copied SES feedback check command");
  };

  return (
    <Shell>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Health</CardTitle>
            <CardDescription>Current notify operational status</CardDescription>
          </CardHeader>
          <CardContent>
            <NotifyStatusBadge status={snapshot.health} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Error rate</CardTitle>
            <CardDescription>
              Failed + bounced deliveries (last 200)
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-2xl">{(snapshot.errorRate * 100).toFixed(1)}%</p>
            <p className="text-muted-foreground text-xs">
              {snapshot.failedDeliveries + snapshot.bouncedDeliveries} /{" "}
              {snapshot.totalDeliveries || 0}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Providers</CardTitle>
            <CardDescription>
              Email provider health and defaults
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-2xl">
              {snapshot.totalProviders - snapshot.unhealthyProviders}/
              {snapshot.totalProviders}
            </p>
            <p className="text-muted-foreground text-xs">
              healthy providers · default email{" "}
              {snapshot.hasDefaultEmailProvider ? "configured" : "missing"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Suppression pressure</CardTitle>
            <CardDescription>
              Suppressed messages in current window
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-2xl">{snapshot.suppressedDeliveries}</p>
            <p className="text-muted-foreground text-xs">
              inactive subscribers {snapshot.inactiveSubscribers}/
              {snapshot.totalSubscribers}
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Operational findings</CardTitle>
            <CardDescription>
              Snapshot-based checks from deliveries, subscribers, providers, and
              policy overrides.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <ul className="list-disc space-y-1 pl-5 text-sm">
              {snapshot.reasons.map((reason) => (
                <li key={reason}>{reason}</li>
              ))}
            </ul>
            <p className="text-muted-foreground text-xs">
              Policy overrides configured: {snapshot.policyOverrides}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Fast actions</CardTitle>
            <CardDescription>
              Jump directly to incident triage views and SES callback checks.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1 text-sm">
              <Link
                className="text-primary underline"
                search={{ status: ["failed", "bounced"] }}
                to="/app/notify/deliveries"
              >
                Open failed and bounced deliveries
              </Link>
              <div>
                <Link
                  className="text-primary underline"
                  to="/app/notify/policies"
                >
                  Review policy overrides
                </Link>
              </div>
              <div>
                <Link
                  className="text-primary underline"
                  to="/app/notify/subscribers"
                >
                  Review subscriber suppression state
                </Link>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={refreshAll} variant="outline">
                Refresh all
              </Button>
              <Button onClick={copyTriageCommand} variant="secondary">
                Copy SES check command
              </Button>
            </div>
            <Textarea
              className="min-h-[96px] font-mono text-xs"
              readOnly
              value={triageCommand}
            />
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
